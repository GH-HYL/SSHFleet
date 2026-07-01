package ssh

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"golang.org/x/crypto/ssh"

	"SSHFleet/internal/log"
)

// SSHClient 封装SSH客户端连接
type SSHClient struct {
	client *ssh.Client
	config *SSHConfig
}

// SSHConfig 存储SSH连接配置
type SSHConfig struct {
	IP             string
	Port           int
	User           string
	Password       string
	ConnectTimeout time.Duration
	ExecTimeout    time.Duration
}

// NewSSHClient 创建SSH客户端实例
func NewSSHClient(config *SSHConfig) *SSHClient {
	return &SSHClient{config: config}
}

// Connect 建立SSH连接
func (c *SSHClient) Connect() error {
	sshClientConfig := &ssh.ClientConfig{
		User:            c.config.User,
		Auth:            []ssh.AuthMethod{ssh.Password(c.config.Password)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         c.config.ConnectTimeout,
	}

	type dialResult struct {
		client *ssh.Client
		err    error
	}
	result := make(chan dialResult, 1)

	go func() {
		addr := fmt.Sprintf("%s:%d", c.config.IP, c.config.Port)
		client, err := ssh.Dial("tcp", addr, sshClientConfig)
		result <- dialResult{client, err}
	}()

	maxTimeout := c.config.ConnectTimeout + 2*time.Second
	select {
	case res := <-result:
		if res.err != nil {
			return fmt.Errorf("建立连接失败 - %w", res.err)
		}
		c.client = res.client
		return nil
	case <-time.After(maxTimeout):
		return fmt.Errorf("建立连接失败 - 握手超时%v", maxTimeout)
	}
}

// ExecuteCommand 执行命令（带上下文中断支持）
func (c *SSHClient) ExecuteCommand(command string, ctx context.Context, ip string) (*ExecResult, error) {
	result := &ExecResult{
		IP:   c.config.IP,
		Port: c.config.Port,
		User: c.config.User,
	}

	// 创建连接
	connStart := time.Now()
	if err := c.Connect(); err != nil {
		result.ConnectCostTime = time.Since(connStart).Seconds()
		result.ConnectSuccess = false
		result.ExitCode = -1
		errMsg := err.Error()
		result.Error = &errMsg
		log.Zlog.Error(fmt.Sprintf("连接失败 - %s: %v", ip, err))
		return result, err
	}
	defer func() { _ = c.Close() }()

	result.ConnectSuccess = true
	result.ConnectCostTime = time.Since(connStart).Seconds()
	log.Zlog.Sugar().Succf("连接成功 - %s", ip)

	// 创建会话
	session, err := c.client.NewSession()
	if err != nil {
		errMsg := err.Error()
		result.Error = &errMsg
		log.Zlog.Error(fmt.Sprintf("会话创建失败 - %s: %v", ip, err))
		return result, fmt.Errorf("创建会话失败 - %w", err)
	}
	log.Zlog.Sugar().Succf("会话成功 - %s", ip)
	defer func() { _ = session.Close() }()

	execStartTime := time.Now()

	// stdout + stderr 合并写入同一个 buffer
	outputBuffer := new(bytes.Buffer)
	session.Stdout = outputBuffer
	session.Stderr = outputBuffer

	// 执行命令（带超时和中断）
	err = c.runWithTimeoutAndCancel(session, command, ctx)

	result.ExecCostTime = time.Since(execStartTime).Seconds()

	// base64 编码输出
	result.Output = base64.StdEncoding.EncodeToString(outputBuffer.Bytes())

	// 处理执行结果
	if err != nil {
		var exitErr *ssh.ExitError
		if errors.As(err, &exitErr) {
			result.ExitCode = exitErr.ExitStatus()
		} else {
			result.ExitCode = -10
			errMsg := err.Error()
			result.Error = &errMsg
		}
	} else {
		result.ExitCode = 0
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
		log.Zlog.Warn(fmt.Sprintf("命令执行超时 - %v", c.config.ExecTimeout))
		return fmt.Errorf("命令执行超时(%v)", c.config.ExecTimeout)
	}
}
