package ssh

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"

	"SSHFleet/internal/log"

	"go.uber.org/zap"
)

// Connect 建立SSH连接
func (c *SSHClient) Connect() error {
	addr := fmt.Sprintf("%s:%d", c.config.IP, c.config.Port)
	log.Zlog.Debug("SSH连接", zap.String("addr", addr))

	authMethods, err := c.config.BuildAuthMethods()
	if err != nil {
		return err
	}

	sshClientConfig := &ssh.ClientConfig{
		User:            c.config.User,
		Auth:            authMethods,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         c.config.ConnectTimeout,
	}

	type dialResult struct {
		client *ssh.Client
		err    error
	}
	result := make(chan dialResult, 1)

	go func() {
		client, err := ssh.Dial("tcp", addr, sshClientConfig)
		result <- dialResult{client, err}
	}()

	maxTimeout := c.config.ConnectTimeout + 2*time.Second
	select {
	case res := <-result:
		if res.err != nil {
			log.Zlog.Error("SSH连接失败", zap.String("user", c.config.User), zap.String("addr", addr), zap.String("登录方式", c.config.authMethodDesc()), zap.Error(res.err))
			return fmt.Errorf("建立连接失败 - %w", res.err)
		}
		c.client = res.client
		log.Zlog.Succ("SSH连接成功", zap.String("user", c.config.User), zap.String("addr", addr), zap.String("登录方式", c.config.authMethodDesc()))
		return nil
	case <-time.After(maxTimeout):
		log.Zlog.Error("SSH连接失败: 握手超时", zap.String("user", c.config.User), zap.String("addr", addr), zap.String("登录方式", c.config.authMethodDesc()), zap.Duration("timeout", maxTimeout))
		return fmt.Errorf("建立连接失败 - 握手超时%v", maxTimeout)
	}
}

// BuildAuthMethods 根据配置构建 SSH 认证方法列表。
// 密钥优先；若密钥解析失败且同时配置了密码，则回退到密码认证。
func (cfg *SSHConfig) BuildAuthMethods() ([]ssh.AuthMethod, error) {
	var authMethods []ssh.AuthMethod

	// 密钥认证（优先）
	if cfg.KeyContent != "" {
		keyBytes := []byte(cfg.KeyContent)
		var (
			signer ssh.Signer
			err    error
		)
		if cfg.KeyPassphrase != "" {
			signer, err = ssh.ParsePrivateKeyWithPassphrase(keyBytes, []byte(cfg.KeyPassphrase))
		} else {
			signer, err = ssh.ParsePrivateKey(keyBytes)
		}
		if err != nil {
			// 密钥解析失败：若同时配置了密码则回退到密码认证，否则直接报错
			if cfg.Password == "" {
				return nil, fmt.Errorf("解析密钥失败 - %w", err)
			}
			log.Zlog.Warn("密钥解析失败，回退到密码认证", zap.Error(err))
		} else {
			authMethods = append(authMethods, ssh.PublicKeys(signer))
		}
	}

	// 密码认证（fallback）
	if cfg.Password != "" {
		authMethods = append(authMethods, ssh.Password(cfg.Password))
	}

	// 防御：至少一种认证方式
	if len(authMethods) == 0 {
		return nil, fmt.Errorf("未提供有效的认证方式（密码或密钥至少提供一种）")
	}

	return authMethods, nil
}

// authMethodDesc 返回本次连接配置的认证方式描述，用于日志标识。
// 密钥优先于密码：同时配置时先尝试密钥，密钥失败回退到密码。
func (cfg *SSHConfig) authMethodDesc() string {
	hasKey := cfg.KeyContent != ""
	hasPwd := cfg.Password != ""
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

// connectSSH 建立 SSH 连接并返回耗时
func (c *SSHClient) connectSSH() connectResult {
	start := time.Now()
	err := c.Connect()
	return connectResult{costTime: time.Since(start).Seconds(), err: err}
}

// ExecuteCommand 执行命令（带上下文中断支持）
func (c *SSHClient) ExecuteCommand(command string, ctx context.Context, ip string) (*ExecResult, error) {
	result := &ExecResult{
		Type: "result",
		IP:   c.config.IP,
		Port: c.config.Port,
		User: c.config.User,
	}

	// 创建连接
	conn := c.connectSSH()
	result.ConnectCostTime = conn.costTime
	if conn.err != nil {
		result.ConnectSuccess = false
		errMsg := conn.err.Error()
		result.Error = &errMsg
		log.Zlog.Error("连接失败", zap.String("ip", ip), zap.String("登录方式", c.config.authMethodDesc()), zap.Error(conn.err))
		return result, conn.err
	}
	defer func() { _ = c.Close() }()

	result.ConnectSuccess = true
	log.Zlog.Succ("连接成功", zap.String("ip", ip), zap.String("登录方式", c.config.authMethodDesc()))

	// 创建会话
	session, err := c.client.NewSession()
	if err != nil {
		errMsg := err.Error()
		result.Error = &errMsg
		log.Zlog.Error("会话创建失败", zap.String("ip", ip), zap.Error(err))
		return result, fmt.Errorf("创建会话失败 - %w", err)
	}
	log.Zlog.Succ("会话成功", zap.String("ip", ip))
	defer func() { _ = session.Close() }()

	execStartTime := time.Now()

	// 使用带锁的写入器，确保 stdout 和 stderr 并发写入安全且保持顺序
	outputWriter := &threadSafeWriter{buf: new(bytes.Buffer)}
	session.Stdout = outputWriter
	session.Stderr = outputWriter

	// 执行命令（带超时和中断）
	err = c.runWithTimeoutAndCancel(session, command, ctx)

	result.ExecCostTime = time.Since(execStartTime).Seconds()

	// base64 编码输出
	rawBytes := outputWriter.buf.Bytes()
	result.Output = base64.StdEncoding.EncodeToString(rawBytes)

	if len(rawBytes) > 0 {
		log.Zlog.Info("节点输出", zap.String("ip", ip), zap.String("output", strings.TrimSpace(string(rawBytes))))
	}

	// 处理执行结果
	if err != nil {
		var exitErr *ssh.ExitError
		if errors.As(err, &exitErr) {
			code := exitErr.ExitStatus()
			result.ExitCode = &code
		} else {
			// 超时或中断等非命令执行错误，靠 error 字段分类
			errMsg := err.Error()
			result.Error = &errMsg
		}
	} else {
		code := 0
		result.ExitCode = &code
	}

	return result, nil
}

// Close 关闭SSH连接
func (c *SSHClient) Close() error {
	if c.client != nil {
		return c.client.Close()
	}
	return nil
}

// runWithTimeoutAndCancel 带超时和中断的命令执行
func (c *SSHClient) runWithTimeoutAndCancel(session *ssh.Session, command string, ctx context.Context) error {
	done := make(chan error, 1)

	go func() {
		done <- session.Run(command)
	}()

	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		_ = session.Close()
		return ctx.Err()
	case <-time.After(c.config.ExecTimeout):
		_ = session.Close()
		log.Zlog.Warn("命令执行超时", zap.Duration("timeout", c.config.ExecTimeout))
		return fmt.Errorf("命令执行超时(%v)", c.config.ExecTimeout)
	}
}

// runCommand 通过 SSH 执行命令
func (c *SSHClient) runCommand(command string) error {
	session, err := c.client.NewSession()
	if err != nil {
		return err
	}
	defer func() { _ = session.Close() }()
	return session.Run(command)
}
