package execute

// 假目标机：测试进程里起一个**真的** SSH 服务端，握手、执行命令、sftp 收发都按协议走。
//
// 为什么不住红线区：红线区只放结构断言，一条行为都不放（ADR-0037 决定二）；而「面板上的
// 完成台数随真实结果动」是行为，只能在自定义区测。ADR-0037 决定六说的「不许与红线区共用
// 假件包」于是不触发——它只被同包的用例引用。
//
// 为什么不用替身插口：`*ssh.Client`／`*sftp.Client` 都是具体类型，伪造不了；与其把传输层
// 接口化，不如让被测的那条路原样跑起来（ADR-0037 决定三留给执行站的那个类型继续空着）。
//
// 射程只有三件事：跑命令、传上去、取下来。改密与代填要 shell、PTY 与提示回放，不做。
//
// 远端根是这台机器上一个临时目录：sftp 服务端用的是 `github.com/pkg/sftp` 自带的那份
// （真实文件系统），所以「远端路径」由 `path()` 拼成 `/C:/…` 这种带前导斜杠的绝对路径，
// 服务端按 Windows／POSIX 各自的规则落回同一个临时目录。

import (
	"crypto/ed25519"
	"crypto/rand"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

// fakeTarget 是一台假目标机：地址是本机的一个端口，远端根是一个临时目录。
type fakeTarget struct {
	addr string
	root string

	ln   net.Listener
	once sync.Once

	// lieSize 枚举时报错的大小（正数加、负数减），lieTimes 是还能骗几次：
	// 0 表示从不骗。核大小那条路只能这么造——服务端用的是真实文件系统，写不短。
	mu       sync.Mutex
	lieSize  int64
	lieTimes int

	// noPrintf 让 find 不认 `-printf`（BusyBox 一类），refuseTimes 是还能拒几次：
	// 0 表示从不拒。用来测枚举的退路。
	refuseTimes int
	// extraRecord 是枚举末尾原样追加的一条记录（名字里带换行时用它钉 NUL 切分）。
	extraRecord string

	// stallSftpOn 置真后 SFTP 服务端只应答版本交换，此后不再应答（发起会一直卡着）。
	// stallExecOn 置真后 exec 请求不回复，客户端一直等（命令发不出去）。
	// 两者共用一个闸门，`stop` 时放开，免得留下永远阻塞的协程。
	stallSftpOn bool
	stallExecOn bool
	stallGate   chan struct{}
}

// lieAboutSize 让接下来的 n 次枚举报出偏移 size 的大小。
func (f *fakeTarget) lieAboutSize(size int64, n int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.lieSize, f.lieTimes = size, n
}

// refusePrintf 让接下来这台的 find 不认 `-printf`：退 1，走 `-print0` 的那条照常应答。
func (f *fakeTarget) refusePrintf() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.refuseTimes = 1 << 30
}

// appendFindRecord 在枚举输出末尾原样追加一条记录，收尾符与本次枚举相同。
func (f *fakeTarget) appendFindRecord(record string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.extraRecord = record
}

// stallSftp 让这台的 SFTP 服务端在版本交换之后不再应答：客户端的下一次发起会一直卡着。
func (f *fakeTarget) stallSftp() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.stallSftpOn = true
	if f.stallGate == nil {
		f.stallGate = make(chan struct{})
	}
}

// stallExec 让这台的 exec 请求不回复：客户端的命令一直等不到应答。
func (f *fakeTarget) stallExec() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.stallExecOn = true
	if f.stallGate == nil {
		f.stallGate = make(chan struct{})
	}
}

// gates 取一次这台的两个闸门状态。
func (f *fakeTarget) gates() (sftpStalled, execStalled bool, gate <-chan struct{}) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.stallSftpOn, f.stallExecOn, f.stallGate
}

// takeFindState 取一次这次的枚举开关：还拒不拒 `-printf`、有没有多余的一条记录。
func (f *fakeTarget) takeFindState() (refuse bool, extra string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.refuseTimes > 0, f.extraRecord
}

// sizeLie 取一次还要不要骗、骗多少；骗过就把次数减掉。
func (f *fakeTarget) sizeLie() (int64, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.lieTimes <= 0 {
		return 0, false
	}
	f.lieTimes--
	return f.lieSize, true
}

// startFakeTarget 起一台，测试结束自动关。
//
// 端口取 `127.0.0.1:0` 由系统分配，不抢固定端口，也不碰网络。
func startFakeTarget(t *testing.T) *fakeTarget {
	t.Helper()

	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("造主机密钥失败：%v", err)
	}
	signer, err := ssh.NewSignerFromKey(priv)
	if err != nil {
		t.Fatalf("主机密钥转签名器失败：%v", err)
	}

	cfg := &ssh.ServerConfig{
		// 收一切登录：假目标机不判身份，身份那一路归真机集成用例。
		PasswordCallback: func(ssh.ConnMetadata, []byte) (*ssh.Permissions, error) { return nil, nil },
	}
	cfg.AddHostKey(signer)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("监听失败：%v", err)
	}

	ft := &fakeTarget{addr: ln.Addr().String(), root: t.TempDir(), ln: ln}
	t.Cleanup(ft.stop)
	go ft.serve(cfg)
	return ft
}

// stop 关掉监听，并放开卡住的闸门；已经在跑的连接由各自的会话收尾。
func (f *fakeTarget) stop() {
	f.once.Do(func() {
		_ = f.ln.Close()
		f.mu.Lock()
		if f.stallGate != nil {
			close(f.stallGate)
			f.stallGate = nil
		}
		f.mu.Unlock()
	})
}

// path 把远端根里的一条相对路径拼成客户端要发的形状：`/C:/…`（Windows）或 `/tmp/…`。
func (f *fakeTarget) path(rel string) string {
	slashed := filepath.ToSlash(filepath.Join(f.root, rel))
	return "/" + strings.TrimPrefix(slashed, "/")
}

// clientConfig 是测试侧连它用的那份配置。
func (f *fakeTarget) clientConfig() *ssh.ClientConfig {
	return &ssh.ClientConfig{
		User:            "tester",
		Auth:            []ssh.AuthMethod{ssh.Password("secret")},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}
}

// dial 连上去，测试结束自动关。
func (f *fakeTarget) dial(t *testing.T) *ssh.Client {
	t.Helper()
	c, err := ssh.Dial("tcp", f.addr, f.clientConfig())
	if err != nil {
		t.Fatalf("连假目标机失败：%v", err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
}

func (f *fakeTarget) serve(cfg *ssh.ServerConfig) {
	for {
		nc, err := f.ln.Accept()
		if err != nil {
			return
		}
		go f.handleConn(nc, cfg)
	}
}

func (f *fakeTarget) handleConn(nc net.Conn, cfg *ssh.ServerConfig) {
	defer func() { _ = nc.Close() }()

	sconn, chans, reqs, err := ssh.NewServerConn(nc, cfg)
	if err != nil {
		return
	}
	defer func() { _ = sconn.Close() }()
	go ssh.DiscardRequests(reqs)

	for nch := range chans {
		if nch.ChannelType() != "session" {
			_ = nch.Reject(ssh.UnknownChannelType, "假目标机只开 session")
			continue
		}
		ch, chReqs, err := nch.Accept()
		if err != nil {
			return
		}
		go f.handleSession(ch, chReqs)
	}
}

func (f *fakeTarget) handleSession(ch ssh.Channel, reqs <-chan *ssh.Request) {
	defer func() { _ = ch.Close() }()

	for req := range reqs {
		switch req.Type {
		case "exec":
			var p struct{ Command string }
			if err := ssh.Unmarshal(req.Payload, &p); err != nil {
				_ = req.Reply(false, nil)
				continue
			}
			// 卡住的那一档：不回复，客户端一直等（命令发不出去）。
			if _, execStalled, gate := f.gates(); execStalled {
				<-gate
			}
			_ = req.Reply(true, nil)
			out, errOut, code := f.run(p.Command)
			if out != "" {
				_, _ = io.WriteString(ch, out)
			}
			if errOut != "" {
				_, _ = io.WriteString(ch.Stderr(), errOut)
			}
			_, _ = ch.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{code}))
			return

		case "subsystem":
			var p struct{ Name string }
			if err := ssh.Unmarshal(req.Payload, &p); err != nil || p.Name != "sftp" {
				_ = req.Reply(false, nil)
				continue
			}
			_ = req.Reply(true, nil)
			var srvCh io.ReadWriteCloser = ch
			if sftpStalled, _, gate := f.gates(); sftpStalled {
				srvCh = &stalledChannel{Channel: ch, gate: gate}
			}
			srv, err := sftp.NewServer(srvCh)
			if err != nil {
				return
			}
			_ = srv.Serve()
			_ = srv.Close()
			return

		case "env", "pty-req", "window-change", "shell":
			_ = req.Reply(true, nil)

		default:
			_ = req.Reply(false, nil)
		}
	}
}

// run 是一条最小命令表：只认这一轮要用的那几条，认不出来的当场报出来，
// 免得将来某条用例悄悄跑在「假目标机什么都不做」上。
//
// 返回 stdout、stderr 与退出码。
func (f *fakeTarget) run(command string) (string, string, uint32) {
	cmd := strings.TrimSpace(command)
	switch {
	case cmd == "" || cmd == "true" || cmd == ":":
		return "", "", 0
	case cmd == "false":
		return "", "", 1
	case strings.HasPrefix(cmd, "echo "):
		return strings.TrimPrefix(cmd, "echo ") + "\n", "", 0
	case cmd == "echo":
		return "\n", "", 0
	case cmd == "uptime":
		return " 10:00:00 up 1 day,  0 users,  load average: 0.00, 0.00, 0.00\n", "", 0
	case strings.HasPrefix(cmd, "mkdir -p "):
		if err := os.MkdirAll(f.local(strings.TrimSpace(strings.TrimPrefix(cmd, "mkdir -p "))), 0o755); err != nil {
			return "", err.Error() + "\n", 1
		}
		return "", "", 0
	case strings.HasPrefix(cmd, "find "):
		return f.find(cmd)
	case strings.HasPrefix(cmd, "test -e "):
		// 下载的远端预检就走这一条（`test -e '<路径>'`）：存在交 0，不存在交 1。
		if _, err := os.Stat(f.local(unquote(strings.TrimPrefix(cmd, "test -e ")))); err != nil {
			return "", "", 1
		}
		return "", "", 0
	case strings.HasPrefix(cmd, "test -d "):
		fi, err := os.Stat(f.local(unquote(strings.TrimPrefix(cmd, "test -d "))))
		if err != nil || !fi.IsDir() {
			return "", "", 1
		}
		return "", "", 0
	}
	return "", fmt.Sprintf("假目标机不认这条命令：%s\n", cmd), 127
}

// find 应答一次枚举。两种形态：
//
//	主路 `find '<路径>' \( -type f -printf 'F %s %P\0' \) -o ...`：一条一条「F 大小 相对路径」；
//	退路 `find '<路径>' \( -type f -print0 \) -o ...`：只给起始点下的完整路径，大小由客户端自己补。
//
// 只造真文件（F）那一半——软链接过滤另有用例。大小照实报，除非 lieAboutSize 让这次说谎。
func (f *fakeTarget) find(command string) (string, string, uint32) {
	rest := strings.TrimPrefix(command, "find ")
	open := strings.IndexByte(rest, '\'')
	if open < 0 {
		return "", "假目标机的 find 要带一个带引号的路径\n", 1
	}
	close := strings.IndexByte(rest[open+1:], '\'')
	if close < 0 {
		return "", "假目标机的 find 路径没闭合\n", 1
	}
	// 用例里的路径是临时目录，不含单引号，按第一段取就够。
	quoted := unquote(rest[open : open+close+2])
	root := f.local(quoted)
	remoteRoot := strings.TrimRight(quoted, "/")

	refuse, extra := f.takeFindState()
	printf := strings.Contains(rest, "-printf")
	if printf && refuse {
		return "", "find: unrecognized: -printf\n", 1
	}
	// 退路只给名字；主路按命令里写的收尾符分隔（`\0` 或 `\n`）。
	namesOnly := !printf
	sep := "\n"
	if namesOnly || strings.Contains(rest, `\0`) {
		sep = "\x00"
	}

	var b strings.Builder
	lie, lying := f.sizeLie()
	err := filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		rel, relErr := filepath.Rel(root, p)
		if relErr != nil {
			return relErr
		}
		rel = filepath.ToSlash(rel)
		if namesOnly {
			b.WriteString(remoteRoot + "/" + rel)
			b.WriteString(sep)
			return nil
		}
		size := info.Size()
		if lying {
			size += lie
		}
		fmt.Fprintf(&b, "F %d %s", size, rel)
		b.WriteString(sep)
		return nil
	})
	if err != nil {
		return "", err.Error() + "\n", 1
	}
	if extra != "" {
		b.WriteString(extra)
		b.WriteString(sep)
	}
	return b.String(), "", 0
}

// sftpInitBytes 版本交换那一个包的字节数：4 字节长度 + 1 字节类型 + 4 字节版本号。
//
// 服务端读满这么多字节就说明握手已经完成；从此不再读，客户端的下一次请求自然没人应答。
const sftpInitBytes = 9

// stallFor 卡住的上限。真实世界里是永远不应答；这里留一个上限，否则客户端关掉 SFTP 通道
// 之后服务端还堵在闸门上、永远看不到 EOF，连「关通道能不能解开阻塞」这件事都测不出来。
const stallFor = 3 * time.Second

// stalledChannel 只放行版本交换的通道：读满 sftpInitBytes 之后停一次，闸门放开或到上限才继续。
type stalledChannel struct {
	ssh.Channel
	seen    int
	stalled bool
	gate    <-chan struct{}
}

func (s *stalledChannel) Read(p []byte) (int, error) {
	if !s.stalled && s.seen >= sftpInitBytes {
		s.stalled = true
		timer := time.NewTimer(stallFor)
		defer timer.Stop()
		select {
		case <-s.gate:
		case <-timer.C:
		}
	}
	n, err := s.Channel.Read(p)
	s.seen += n
	return n, err
}

// local 把客户端发来的远端路径落回本机路径（`/C:/…` → `C:\…`）。
func (f *fakeTarget) local(remote string) string {
	p := strings.TrimPrefix(remote, "/")
	if len(p) >= 2 && p[1] == ':' {
		return filepath.FromSlash(p)
	}
	return filepath.FromSlash("/" + p)
}

// unquote 脱掉命令里那层单引号：下发行按 POSIX 规则转义过（`test -e '<路径>'`）。
func unquote(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 && s[0] == '\'' && s[len(s)-1] == '\'' {
		return strings.ReplaceAll(s[1:len(s)-1], `'\''`, `'`)
	}
	return s
}
