package ssh

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

// handshakeGrace 握手宽限：对位旧实现 ClientConfig.Timeout + time.After(T+2s) 的双机制
// （dial 用 timeout_connect，握手再给 2 秒宽限）。差别是新实现用连接 deadline 表达，
// 不再起 goroutine + timer。
const handshakeGrace = 2 * time.Second

// Client 单个节点的 SSH 客户端（有生命周期：Connect → 使用 → Close，spec D32 条件 2）。
type Client struct {
	cfg  *Config
	conn *ssh.Client

	// publicKeyOffered 本次连接是否真的把公钥认证挂进了认证列表
	//（私钥解析失败会回退密码，此时不算「两种都试过」）
	publicKeyOffered bool

	// banner 认证阶段服务端下发的提示原文（SSH_MSG_USERAUTH_BANNER）。
	// 账号过期、密码必须修改、/etc/nologin 通知等都只在这里出现——x/crypto 在没有
	// BannerCallback 时会把它整包丢弃，所以必须自己接住（ADR-0005）。
	banner string
}

func NewClient(cfg *Config) *Client { return &Client{cfg: cfg} }

// Connect 建立连接：拨号超时 = ConnectTimeout，握手宽限 +2 秒；主机密钥跳过校验。
func (c *Client) Connect(ctx context.Context) error {
	addr := net.JoinHostPort(c.cfg.IP, fmt.Sprintf("%d", c.cfg.Port))

	c.banner = "" // 重连时清掉上一次的提示，避免叠加

	authMethods, err := c.buildAuthMethods()
	if err != nil {
		return err
	}
	sshCfg := &ssh.ClientConfig{
		User:            c.cfg.User,
		Auth:            authMethods,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), // ADR-0001：刻意跳过
		// 接住服务端提示（ADR-0005）。回调在握手阶段被调用，即使紧接着连接被服务端
		// 断开（账号过期就是这种），提示也已经到手。
		BannerCallback: func(msg string) error {
			c.banner += msg
			return nil
		},
	}

	dialer := net.Dialer{Timeout: c.cfg.ConnectTimeout}
	rawConn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return fmt.Errorf("建立连接失败 - %w", err)
	}

	// 握手阶段限时（拨号 + 宽限）；成功后清掉 deadline，避免影响后续会话
	_ = rawConn.SetDeadline(time.Now().Add(c.cfg.ConnectTimeout + handshakeGrace))
	sshConn, chans, reqs, err := ssh.NewClientConn(rawConn, addr, sshCfg)
	if err != nil {
		_ = rawConn.Close()
		if errors.Is(err, context.DeadlineExceeded) || isTimeout(err) {
			return fmt.Errorf("建立连接失败 - 握手超时%v", c.cfg.ConnectTimeout+handshakeGrace)
		}
		return fmt.Errorf("建立连接失败 - %w", err)
	}
	_ = rawConn.SetDeadline(time.Time{})
	c.conn = ssh.NewClient(sshConn, chans, reqs)
	return nil
}

func isTimeout(err error) bool {
	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Timeout()
}

// Close 关闭连接（幂等）。
func (c *Client) Close() error {
	if c.conn != nil {
		err := c.conn.Close()
		c.conn = nil
		return err
	}
	return nil
}

// buildAuthMethods 认证方法列表：密钥就只走密钥，密码就只走密码。
// 密钥解析失败即失败——不回退密码：配了密钥却在背后偷偷用密码登录，
// 会让人以为密钥是好的，直到密码也失效那天才发现。
func (c *Client) buildAuthMethods() ([]ssh.AuthMethod, error) {
	var methods []ssh.AuthMethod

	if c.cfg.KeyContent != "" {
		keyBytes := []byte(c.cfg.KeyContent)
		var (
			signer ssh.Signer
			err    error
		)
		if c.cfg.KeyPassphrase != "" {
			signer, err = ssh.ParsePrivateKeyWithPassphrase(keyBytes, []byte(c.cfg.KeyPassphrase))
		} else {
			signer, err = ssh.ParsePrivateKey(keyBytes)
		}
		if err != nil {
			var missing *ssh.PassphraseMissingError
			if errors.As(err, &missing) {
				return nil, fmt.Errorf(
					"这把私钥有口令，没有给口令\n" +
						"提示：在第 6 列写口令，或把默认口令配到配置文件 credential.key_password")
			}
			return nil, fmt.Errorf("解析密钥失败 - %w", err)
		}
		methods = append(methods, ssh.PublicKeys(signer))
		c.publicKeyOffered = true
	}

	if c.cfg.Password != "" {
		methods = append(methods, ssh.Password(c.cfg.Password))
	}

	if len(methods) == 0 {
		return nil, fmt.Errorf("未提供有效的认证方式（密码或密钥至少提供一种）")
	}
	return methods, nil
}

// applyBanner 把认证阶段的服务端提示并入结果的报错原文（ADR-0005）。
//
// 为什么并入原文而不是单开字段：分类判据全都在原文上做匹配，提示进了原文就自然
// 参与匹配，不必为它再改判据表、统计、终端、报告与 xlsx 的字段口径。
//
// 为什么不追加到"干净成功"的行：提示可能只是 MOTD / 法务声明这类公告，把它写进
// 正常行的报错列会造成误读。失败的、部分失败的行才追加。
func (c *Client) applyBanner(result *Result) {
	tip := strings.TrimSpace(c.banner)
	if tip == "" || isCleanSuccess(result) {
		return
	}
	if result.Error == nil || *result.Error == "" {
		result.Error = strPtr(tip)
		return
	}
	*result.Error = *result.Error + "\n" + tip
}

// isCleanSuccess 干净成功：连接成功、退出码为 0、且没有失败文件（传输模式）。
// 注意不能只看报错字段——命令模式成功时退出码为 0，与报错字段无关。
func isCleanSuccess(r *Result) bool {
	return r.ConnectSuccess && r.ExitCode != nil && *r.ExitCode == 0 && r.FailedFiles == 0
}

// authMethodDesc 认证方式描述（执行期日志的「登录方式」）。
func (c *Client) authMethodDesc() string {
	hasKey := c.cfg.KeyContent != ""
	hasPwd := c.cfg.Password != ""
	switch {
	case hasKey:
		return "密钥"
	case hasPwd:
		return "密码"
	default:
		return "未配置"
	}
}

// runCommand 执行一条辅助命令（sudo 前置清理、临时目录、mv、test 等），不采集输出。
func (c *Client) runCommand(command string) error {
	session, err := c.conn.NewSession()
	if err != nil {
		return err
	}
	defer func() { _ = session.Close() }()
	return session.Run(command)
}

// runCommandCapture 执行辅助命令并返回合并输出（用于取远程文件清单）。
func (c *Client) runCommandCapture(command string) (string, error) {
	session, err := c.conn.NewSession()
	if err != nil {
		return "", err
	}
	defer func() { _ = session.Close() }()
	out, err := session.CombinedOutput(command)
	return string(out), err
}
