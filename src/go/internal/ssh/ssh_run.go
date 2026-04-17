package ssh

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"

	"sshexec_go/internal/log"
)

// 封装SSH客户端连接
type SSHClient struct {
	client *ssh.Client
	config *SSHConfig
}

// SSHConfig 存储SSH连接配置
type SSHConfig struct {
	IP             string        // 目标服务器IP
	Port           int           // SSH端口
	User           string        // 登录用户名
	Password       string        // 登录密码
	PrivateKeyPath string        // 私钥路径
	ConnectTimeout time.Duration // 连接超时时间
	ExecTimeout    time.Duration // 命令执行超时时间
}

// 创建SSH客户端实例
func NewSSHClient(config *SSHConfig) *SSHClient {
	return &SSHClient{config: config}
}

// 建立SSH连接
func (c *SSHClient) Connect() error {
	sshClientConfig := &ssh.ClientConfig{
		User:            c.config.User,
		Auth:            []ssh.AuthMethod{},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         c.config.ConnectTimeout,
	}

	// 添加认证方式
	var keyAuth ssh.AuthMethod
	var err error
	// 私钥登陆
	if c.config.PrivateKeyPath != "" {
		keyAuth, err = c.loadPrivateKey()
		if err != nil {
			return fmt.Errorf("私钥认证失败: %w", err)
		}
		if keyAuth != nil {
			//log.Zlog.Sugar().Debugf("私钥认证 - 采用私钥登陆，IP：%s", c.config.IP)
			sshClientConfig.Auth = append(sshClientConfig.Auth, keyAuth)
		}
	}
	// 密码登陆
	if keyAuth == nil {
		sshClientConfig.Auth = append(sshClientConfig.Auth, ssh.Password(c.config.Password))
	}
	//
	if len(sshClientConfig.Auth) == 0 {
		return errors.New("未配置认证方式（密码或私钥）")
	}

	type dialResult struct {
		client *ssh.Client
		err    error
	}
	result := make(chan dialResult)

	// 建立连接，结果返回通道
	go func() {
		addr := fmt.Sprintf("%s:%d", c.config.IP, c.config.Port)
		client, err := ssh.Dial("tcp", addr, sshClientConfig)
		result <- dialResult{client, err}
	}()

	maxTimeout := c.config.ConnectTimeout + 2*time.Second
	select {
	case result := <-result:
		if result.err != nil {
			return fmt.Errorf("建立连接失败 - %w", result.err)
		}
		c.client = result.client
		return nil
	case <-time.After(maxTimeout):
		return fmt.Errorf("建立连接失败 - 握手超时%v", maxTimeout)

	}
}

// 执行命令（带上下文中断支持）
func (c *SSHClient) ExecuteCommand(command string, ctx context.Context, ip string) (*ExecResult, error) {
	result := &ExecResult{
		IP:   c.config.IP,
		Port: c.config.Port,
		User: c.config.User,
	}

	// 创建连接
	connStart := time.Now()
	if err := c.Connect(); err != nil {
		result.ConnCostTime = time.Since(connStart).Seconds()
		result.ConnectSuccess = false
		result.ConnectMsg = err.Error()
		result.ResultCategory = ClassifyError(err)
		return result, err
	}
	defer func(c *SSHClient) {
		err := c.Close()
		if err != nil {
		}
	}(c)

	result.ConnectSuccess = true
	result.ConnectMsg = "连接成功"
	result.ConnCostTime = time.Since(connStart).Seconds()
	log.Zlog.Sugar().Succf("连接成功 - %s", ip)

	// 创建会话
	session, err := c.client.NewSession()
	if err != nil {
		result.ResultCategory = ClassifyError(err)
		return result, fmt.Errorf("创建会话失败 - %w", err)
	}
	log.Zlog.Sugar().Succf("会话成功 - %s", ip)
	defer func(session *ssh.Session) {
		err := session.Close()
		if err != nil {
		}
	}(session)

	execStartTime := time.Now()

	// 创建一个缓冲器用于接收输出
	outputBuffer := new(bytes.Buffer)

	// 设置session的输出，关键：stdout、stderr写入时同一哥writer
	combineWriter := io.MultiWriter(outputBuffer)
	session.Stdout = combineWriter
	session.Stderr = combineWriter

	// 执行命令（带超时和中断）
	err = c.runWithTimeoutAndCancel(session, command, ctx)

	// 执行时间统计
	result.ExecCostTime = time.Since(execStartTime).Seconds()

	// 返回结果输出值
	output := strings.TrimSpace(outputBuffer.String())
	output = strings.Replace(output, "\\ufffd", "/ufffd", -1)
	ansiRegex := regexp.MustCompile(`\x1b\[[0-?]*[ -/]*[@-~]`)
	result.Output = ansiRegex.ReplaceAllString(output, "")

	// 处理执行结果
	if err != nil {
		var exitErr *ssh.ExitError
		// 判断是不是ssh的专属错误
		if errors.As(err, &exitErr) {
			result.ExitCode = exitErr.ExitStatus()
			result.ResultCategory = fmt.Sprintf("退出码-%d", result.ExitCode)
			if result.ExitCode == 1 && strings.Contains(result.Output, "WARNING: Your password has expired.") {
				result.ResultCategory = "密码过期"
			}
		} else {
			result.ExitCode = -10
			result.ResultCategory = ClassifyError(err)
		}
		result.ExecBool = false
	} else {
		result.ExitCode = 0
		result.ResultCategory = "执行成功"
		result.ExecBool = true

	}

	return result, nil
}

// 关闭SSH连接
func (c *SSHClient) Close() error {
	if c.client != nil {
		return c.client.Close()
	}
	return nil
}

// 加载私钥
func (c *SSHClient) loadPrivateKey() (ssh.AuthMethod, error) {
	keyData, err := os.ReadFile(c.config.PrivateKeyPath)
	if err != nil {
		return nil, fmt.Errorf("读取私钥文件失败 - %w", err)
	}

	var signer ssh.Signer
	// 无密码的私钥认证
	signer, err = ssh.ParsePrivateKey(keyData)
	if err != nil {
		log.Zlog.Sugar().Errorf("私钥认证失败 - %s", err)
		return nil, nil
	}

	//// 有密码的私钥认证
	//if c.config.PrivateKeyPassphrase != "" {
	//	signer, err = ssh.ParsePrivateKeyWithPassphrase(keyData, []byte(c.config.PrivateKeyPassphrase))
	//	if err != nil {
	//		return nil, fmt.Errorf("私钥密码解析失败 - %w", err)
	//	}
	//}
	return ssh.PublicKeys(signer), nil
}

// 带超时和中断的命令执行
func (c *SSHClient) runWithTimeoutAndCancel(session *ssh.Session, command string, ctx context.Context) error {

	// 创建Run的完成通道
	done := make(chan error, 1)

	// 执行命令
	go func() {
		err := session.Run(command)
		done <- err
	}()

	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		_ = session.Close() // 强制关闭会话
		return ctx.Err()
	case <-time.After(c.config.ExecTimeout):
		_ = session.Close()
		return fmt.Errorf("命令执行超时(%v)", c.config.ExecTimeout)
	}
}
