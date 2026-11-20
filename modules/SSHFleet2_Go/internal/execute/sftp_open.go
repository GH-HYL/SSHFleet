package execute

// 提权 SFTP 会话：`--sudo` 时整条通道以 root 跑（ADR-0063 决定 1、2）。
//
// 形态：一条 exec 会话探测 `sftp-server` 的绝对路径，再起一条 `sudo -n <路径>`，把它的
// stdout／stdin 交给 SFTP 客户端。此后所有读写都由那个 root 进程执行，落位文件的属主
// 随之变成 root（ADR-0063 决定 1 已接受）。
//
// 为什么不再走暂存目录：暂存要 `chmod 777`（期间任何账号都能写）、跨分区时 `mv` 退化成
// 复制、还要多一条 `/tmp` 空间判据——三处补丁都是绕路的产物，不是需求。

import (
	"errors"
	"strings"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"

	"sshfleet2/internal/criterion"
)

// sftpServerProbe 探测 sftp-server 的路径：先试三个常见位置，再退到 PATH。
//
// 路径不写死：目标机是几千台不同发行版，任何一处假设不成立，现象都是「某几台莫名其妙
// 失败」（ADR-0063 决定 2）。找到就打印路径并退出 0，找不到退出 127。
const sftpServerProbe = `for p in /usr/lib/openssh/sftp-server /usr/libexec/openssh/sftp-server /usr/lib/ssh/sftp-server; do
	if [ -x "$p" ]; then printf '%s\n' "$p"; exit 0; fi
done
p=$(command -v sftp-server 2>/dev/null) || exit 127
[ -n "$p" ] && printf '%s\n' "$p" && exit 0
exit 127
`

// sftpConn 一条 SFTP 通道：客户端，以及提权路径下它底下那条会话。
//
// 非提权路径没有独立会话（客户端直接挂在 SSH 连接上），那一格留空。
type sftpConn struct {
	*sftp.Client
	sess *ssh.Session
}

// Close 关客户端；提权路径下连那条会话一起关，否则漏一条通道。
func (s *sftpConn) Close() error {
	err := s.Client.Close()
	if s.sess != nil {
		_ = s.sess.Close()
	}
	return err
}

// openSFTP 开一条读写通道。useSudo 时整条会话以 root 跑。
func (c *client) openSFTP(useSudo bool) (*sftpConn, error) {
	if !useSudo {
		client, err := sftp.NewClient(c.conn)
		if err != nil {
			return nil, err
		}
		return &sftpConn{Client: client}, nil
	}

	server, err := c.probeSFTPServer()
	if err != nil {
		return nil, err
	}
	return c.startSFTPServer(server)
}

// probeSFTPServer 问一次目标机：sudo 能把哪个 sftp-server 跑起来。
//
// 两种失败分开说：探不到（退出 127）与 sudo 拒绝（其余非 0）。都挂 `client_failed`——
// 这一类连「开始传」都做不到，与「目标机不支持 SFTP」是同一档。
func (c *client) probeSFTPServer() (string, error) {
	sess, err := c.conn.NewSession()
	if err != nil {
		return "", errors.New(msg(envTransfer, transferClientFailed, "open session for sudo -n: "+err.Error()))
	}
	defer func() { _ = sess.Close() }()

	// 探测脚本走 stdin：免得把多行脚本套进引号里，一层转义写错就是「某几台莫名其妙失败」。
	sess.Stdin = strings.NewReader(sftpServerProbe)
	out, err := sess.CombinedOutput("sudo -n sh")
	text := strings.TrimSpace(string(out))

	switch {
	case err == nil && text != "":
		return text, nil
	case err == nil || isExitStatus(err, 127):
		return "", errors.New(msg(envTransfer, transferClientFailed, "sftp-server not found under sudo"+detailOf(text)))
	default:
		return "", errors.New(msg(envTransfer, transferClientFailed, "sudo refused to run sftp-server"+detailOf(text)))
	}
}

// startSFTPServer 起 `sudo -n <server>`，把它的 stdout／stdin 交给 SFTP 客户端。
func (c *client) startSFTPServer(server string) (*sftpConn, error) {
	sess, err := c.conn.NewSession()
	if err != nil {
		return nil, errors.New(msg(envTransfer, transferClientFailed, "open session for sftp-server: "+err.Error()))
	}

	in, err := sess.StdinPipe()
	if err != nil {
		_ = sess.Close()
		return nil, errors.New(msg(envTransfer, transferClientFailed, "sftp-server stdin: "+err.Error()))
	}
	out, err := sess.StdoutPipe()
	if err != nil {
		_ = sess.Close()
		return nil, errors.New(msg(envTransfer, transferClientFailed, "sftp-server stdout: "+err.Error()))
	}
	// 起不来时远端说的话在 stderr 上：收住它，握手失败时一并报出来。
	stderr := &strings.Builder{}
	sess.Stderr = stderr

	if err := sess.Start("sudo -n " + criterion.SingleQuoted(server)); err != nil {
		_ = sess.Close()
		return nil, errors.New(msg(envTransfer, transferClientFailed, "start sftp-server: "+err.Error()))
	}

	client, err := sftp.NewClientPipe(out, in)
	if err != nil {
		_ = sess.Close()
		return nil, errors.New(msg(envTransfer, transferClientFailed,
			"sftp-server handshake: "+err.Error()+detailOf(strings.TrimSpace(stderr.String()))))
	}
	return &sftpConn{Client: client, sess: sess}, nil
}

// isExitStatus 判远端命令是不是以某个退出码收场。
func isExitStatus(err error, code int) bool {
	var exitErr *ssh.ExitError
	return errors.As(err, &exitErr) && exitErr.ExitStatus() == code
}

// detailOf 把远端原话接在报文后面（空串就什么都不加）。
func detailOf(text string) string {
	if text == "" {
		return ""
	}
	return ": " + text
}
