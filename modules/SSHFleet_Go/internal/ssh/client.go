package ssh

import (
	"context"
	"errors"
	"fmt"
	"net"
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
}

func NewClient(cfg *Config) *Client { return &Client{cfg: cfg} }

// Connect 建立连接：拨号超时 = ConnectTimeout，握手宽限 +2 秒；主机密钥跳过校验。
func (c *Client) Connect(ctx context.Context) error {
	addr := net.JoinHostPort(c.cfg.IP, fmt.Sprintf("%d", c.cfg.Port))

	authMethods, err := c.buildAuthMethods()
	if err != nil {
		return err
	}
	sshCfg := &ssh.ClientConfig{
		User:            c.cfg.User,
		Auth:            authMethods,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), // ADR-0001：刻意跳过
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

// buildAuthMethods 认证方法列表：密钥优先；密钥解析失败且配了密码才回退密码
// （直接沿用旧 Go 实现的语义）。
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
			if c.cfg.Password == "" {
				return nil, fmt.Errorf("解析密钥失败 - %w", err)
			}
			// 密钥解析失败但配了密码：回退密码认证（旧行为）
		} else {
			methods = append(methods, ssh.PublicKeys(signer))
		}
	}

	if c.cfg.Password != "" {
		methods = append(methods, ssh.Password(c.cfg.Password))
	}

	if len(methods) == 0 {
		return nil, fmt.Errorf("未提供有效的认证方式（密码或密钥至少提供一种）")
	}
	return methods, nil
}

// authMethodDesc 认证方式描述（日志口径与旧一致）。
func (c *Client) authMethodDesc() string {
	hasKey := c.cfg.KeyContent != ""
	hasPwd := c.cfg.Password != ""
	switch {
	case hasKey && hasPwd:
		return "密钥/密码"
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
