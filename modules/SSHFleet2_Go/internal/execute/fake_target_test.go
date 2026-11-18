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

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

// fakeTarget 是一台假目标机：地址是本机的一个端口，远端根是一个临时目录。
type fakeTarget struct {
	addr string
	root string

	ln   net.Listener
	once sync.Once
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

// stop 关掉监听；已经在跑的连接由各自的会话收尾。
func (f *fakeTarget) stop() {
	f.once.Do(func() { _ = f.ln.Close() })
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
			srv, err := sftp.NewServer(ch)
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
