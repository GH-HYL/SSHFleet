package execute

// 一台机器的 SSH 连接：拨号、认证、收服务端提示，然后关掉。
//
// 与老版 `internal/ssh/client.go` 的差别只有两处，都在这一单的纸面上：
//   - 私钥由**路径**读成原文（纸上带的是私钥文件路径，老版在清单那步就读成原文了）；
//   - 超时从纸上「执行参数」那一格取，不在这里写死。
//
// 主机密钥刻意跳过校验（ADR-0001）；服务端提示必须接住（ADR-0005）——账号过期、
// 密码必须修改、`/etc/nologin` 通知都只从 banner 出来，`x/crypto` 在没有回调时会把它整包丢弃。

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"

	"sshfleet2/internal/contract"
)

// handshakeGrace 握手宽限：拨号按纸上那个连接超时，握手再给 2 秒
// （对位老版 `ClientConfig.Timeout` + `time.After(T+2s)` 的双机制，这里用连接 deadline 表达，
// 不起 goroutine 也不起 timer）。
const handshakeGrace = 2 * time.Second

// client 一台机器的连接。有生命周期：connect → 使用 → close。
type client struct {
	node    contract.Node
	timeout time.Duration
	conn    *ssh.Client

	// banner 认证阶段服务端下发的提示原文（SSH_MSG_USERAUTH_BANNER），重连时清空。
	banner string
}

// connect 建立连接：拨号超时 = 纸上那个连接超时，握手宽限 +2 秒；主机密钥跳过校验。
//
// context 真的递到拨号上：传一个已取消的进来，建连立刻失败，不留后台 goroutine。
func (c *client) connect(ctx context.Context) error {
	addr := net.JoinHostPort(c.node.IP, c.node.Port)
	c.banner = "" // 重连时清掉上一次的提示，避免叠加

	authMethods, err := c.authMethods()
	if err != nil {
		return err
	}

	sshCfg := &ssh.ClientConfig{
		User:            c.node.User,
		Auth:            authMethods,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), // ADR-0001：刻意跳过
		// 接住服务端提示（ADR-0005）。回调在握手阶段被调用，即使紧接着连接被服务端断开
		// （账号过期就是这种），提示也已经到手。
		BannerCallback: func(text string) error {
			c.banner += text
			return nil
		},
	}

	dialer := net.Dialer{Timeout: c.timeout}
	rawConn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return errors.New(connectMessage(connectStageOfDialError(err), err))
	}

	// 握手阶段限时（拨号 + 宽限）；成功后清掉 deadline，免得影响后续会话。
	_ = rawConn.SetDeadline(time.Now().Add(c.timeout + handshakeGrace))
	sshConn, chans, reqs, err := ssh.NewClientConn(rawConn, addr, sshCfg)
	if err != nil {
		_ = rawConn.Close()
		if errors.Is(err, context.DeadlineExceeded) || isTimeout(err) {
			return errors.New(msg(envConnect, connectHandshakeTimeout,
				fmt.Sprintf("handshake not completed within %v", c.timeout+handshakeGrace)))
		}
		return errors.New(connectMessage(connectStageOfHandshakeError(err), err))
	}
	_ = rawConn.SetDeadline(time.Time{})
	c.conn = ssh.NewClient(sshConn, chans, reqs)
	return nil
}

// close 关闭连接（幂等）。
func (c *client) close() {
	if c.conn != nil {
		_ = c.conn.Close()
		c.conn = nil
	}
}

// errorText 是这一台进「报错原文」那一格的文本：原始错误在前、服务端提示在后。
//
// 为什么提示并进这一格、不另立一格：`contract.NodeResult.ErrorText` 的注释写着「工具自己
// 那条报文、远端原话、服务端提示都原样在里面」，判定按这一格撞关键词就能认出「账号过期」
// 这类原因；而屏幕上只在失败行显示它（正常行不显示）——与老版把 `Error` 与 `ServerBanner`
// 合成显示的观感一致，只是两者从此只有一个载体。
func (c *client) errorText(err error) string {
	parts := make([]string, 0, 2)
	if err != nil {
		parts = append(parts, err.Error())
	}
	if hint := strings.TrimSpace(c.banner); hint != "" {
		parts = append(parts, hint)
	}
	return strings.Join(parts, "\n")
}

// authMethods 认证方法列表：密钥就只走密钥，密码就只走密码（两者都配时按老版顺序：先密钥后密码）。
//
// 私钥解析失败即失败——**不回退密码**：配了密钥却在背后偷偷用密码登录，会让人以为密钥是好的，
// 直到密码也失效那天才发现。
//
// 私钥文件读不到也挂 `key_parse_failed`：名字说的是「哪一步、什么现象」，私钥拿不到就是这把
// 私钥用不成；不新造环节名，免得判据表里那张关键词认不出它、把这一类落到「未归类」。
// （老版在清单那步就把 PEM 读出来验过，报得比这里早一环；见本单 `## Comments` 的已知差异。）
func (c *client) authMethods() ([]ssh.AuthMethod, error) {
	var methods []ssh.AuthMethod

	if c.node.Key != "" {
		keyBytes, err := os.ReadFile(c.node.Key)
		if err != nil {
			return nil, errors.New(connectMessage(connectKeyParseFailed, err))
		}
		var signer ssh.Signer
		if c.node.KeyPassword != "" {
			signer, err = ssh.ParsePrivateKeyWithPassphrase(keyBytes, []byte(c.node.KeyPassword))
		} else {
			signer, err = ssh.ParsePrivateKey(keyBytes)
		}
		if err != nil {
			var missing *ssh.PassphraseMissingError
			if errors.As(err, &missing) {
				return nil, errors.New(msg(envConnect, connectKeyNoPassphrase,
					"the private key is encrypted but no passphrase was given"))
			}
			return nil, errors.New(connectMessage(connectKeyParseFailed, err))
		}
		methods = append(methods, ssh.PublicKeys(signer))
	}

	if c.node.Password != "" {
		methods = append(methods, ssh.Password(c.node.Password))
	}

	if len(methods) == 0 {
		// 老版原话，一个字不改：这一句不是报文（它没有环节名好挂），是工具在说「没法开认证」。
		return nil, fmt.Errorf("未提供有效的认证方式（密码或密钥至少提供一种）")
	}
	return methods, nil
}

// isTimeout 判一个错误是不是「到点了」。
func isTimeout(err error) bool {
	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Timeout()
}
