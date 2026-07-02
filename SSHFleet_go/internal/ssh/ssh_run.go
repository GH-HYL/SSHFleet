package ssh

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"os"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"

	"SSHFleet/internal/log"
	"SSHFleet/internal/localfs"
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

// UploadFiles 上传文件到远程节点
func (c *SSHClient) UploadFiles(
	fileItems []localfs.FileItem,
	remotePath string,
	useSudo bool,
	ctx context.Context,
	ip string,
) (*UploadResult, error) {
	result := &UploadResult{
		IP:         c.config.IP,
		Port:       c.config.Port,
		User:       c.config.User,
		TotalFiles: len(fileItems),
		Files:      make([]FileUploadItem, 0, len(fileItems)),
	}

	// 1. SSH 建连
	connStart := time.Now()
	if err := c.Connect(); err != nil {
		result.ConnectCostTime = time.Since(connStart).Seconds()
		result.ConnectSuccess = false
		errMsg := err.Error()
		result.Error = &errMsg
		log.Zlog.Error(fmt.Sprintf("[上传] 连接失败 - %s: %v", ip, err))
		return result, err
	}
	defer func() { _ = c.Close() }()

	result.ConnectSuccess = true
	result.ConnectCostTime = time.Since(connStart).Seconds()
	log.Zlog.Succ(fmt.Sprintf("[上传] 连接成功 - %s", ip))

	// 2. 清理残留临时目录（仅 sudo 模式）
	if useSudo {
		c.runCommand(fmt.Sprintf("sudo rm -rf /tmp/.SSHFleet_tmp/"))
		log.Zlog.Info(fmt.Sprintf("[上传] 清理残留临时目录 - %s", ip))
	}

	// 3. 创建 SFTP 审户端
	sftpClient, err := sftp.NewClient(c.client)
	if err != nil {
		errMsg := err.Error()
		result.Error = &errMsg
		log.Zlog.Error(fmt.Sprintf("[上传] SFTP 客户端创建失败 - %s: %v", ip, err))
		return result, err
	}
	defer func() { _ = sftpClient.Close() }()
	log.Zlog.Info(fmt.Sprintf("[上传] SFTP 客户端创建成功 - %s", ip))

	// 4. 检查远程目标路径
	fi, err := sftpClient.Stat(remotePath)
	if err != nil {
		errMsg := fmt.Sprintf("远程目标路径不存在: %s", remotePath)
		result.Error = &errMsg
		log.Zlog.Error(fmt.Sprintf("[上传] %s - %s", ip, errMsg))
		return result, fmt.Errorf("%s", errMsg)
	}
	if !fi.IsDir() {
		errMsg := fmt.Sprintf("远程目标路径不是目录: %s", remotePath)
		result.Error = &errMsg
		log.Zlog.Error(fmt.Sprintf("[上传] %s - %s", ip, errMsg))
		return result, fmt.Errorf("%s", errMsg)
	}
	log.Zlog.Info(fmt.Sprintf("[上传] 远程目标路径检查通过 - %s: %s", ip, remotePath))

	// 5. 判断 sudo 是否实际生效
	effectiveSudo := useSudo && c.config.User != "root"
	if useSudo && c.config.User == "root" {
		log.Zlog.Info(fmt.Sprintf("[上传] 用户已是 root，跳过 sudo - %s", ip))
	}

	// 6. 逐文件处理
	for _, item := range fileItems {
		select {
		case <-ctx.Done():
			errMsg := "上传超时被取消"
			result.Error = &errMsg
			log.Zlog.Error(fmt.Sprintf("[上传] %s - %s", ip, errMsg))
			return result, ctx.Err()
		default:
		}

		fileStart := time.Now()
		fileResult := FileUploadItem{
			FileName:   item.FileName,
			FilePath:   item.LocalPath,
			FileSize:   item.FileSize,
		}

		// 6a. 检查远程文件是否已存在
		remoteFilePath := remotePath + "/" + item.FileName
		if _, err := sftpClient.Stat(remoteFilePath); err == nil {
			errMsg := "文件已存在"
			fileResult.Error = &errMsg
			fileResult.Success = false
			result.FailedFiles++
			result.Files = append(result.Files, fileResult)
			log.Zlog.Warn(fmt.Sprintf("[上传] 文件已存在，跳过 - %s: %s", ip, remoteFilePath))
			continue
		}

		// 6b. 读取本地文件权限
		localInfo, err := os.Stat(item.LocalPath)
		if err != nil {
			errMsg := fmt.Sprintf("读取本地文件失败: %v", err)
			fileResult.Error = &errMsg
			fileResult.Success = false
			result.FailedFiles++
			result.Files = append(result.Files, fileResult)
			log.Zlog.Error(fmt.Sprintf("[上传] %s - %s: %s", ip, item.FileName, errMsg))
			continue
		}
		localMode := localInfo.Mode().Perm()

		// 6c. 执行上传
		var uploadErr error
		if !effectiveSudo {
			// 直接写入目标路径
			uploadErr = c.sftpUploadFile(sftpClient, item.LocalPath, remoteFilePath, localMode, ip)
		} else {
			// 写入临时目录 + sudo mv
			uploadErr = c.sftpUploadWithSudo(sftpClient, item.LocalPath, item.FileName, remotePath, localMode, ip)
		}

		fileResult.CostTime = time.Since(fileStart).Seconds()

		if uploadErr != nil {
			errMsg := uploadErr.Error()
			fileResult.Error = &errMsg
			fileResult.Success = false
			result.FailedFiles++
			log.Zlog.Error(fmt.Sprintf("[上传] 文件上传失败 - %s: %s: %v", ip, item.FileName, uploadErr))
		} else {
			fileResult.Success = true
			result.SuccessFiles++
			log.Zlog.Info(fmt.Sprintf("[上传] 文件上传成功 - %s: %s (%.3fs)", ip, item.FileName, fileResult.CostTime))
		}

		result.Files = append(result.Files, fileResult)
	}

	log.Zlog.Succ(fmt.Sprintf("[上传] 节点完成 - %s: 成功 %d/%d", ip, result.SuccessFiles, result.TotalFiles))
	return result, nil
}

// sftpUploadFile 直接通过 SFTP 写入文件
func (c *SSHClient) sftpUploadFile(sftpClient *sftp.Client, localPath, remoteFilePath string, perm os.FileMode, ip string) error {
	srcFile, err := os.Open(localPath)
	if err != nil {
		return fmt.Errorf("打开本地文件失败: %w", err)
	}
	defer func() { _ = srcFile.Close() }()

	dstFile, err := sftpClient.Create(remoteFilePath)
	if err != nil {
		return fmt.Errorf("创建远程文件失败: %w", err)
	}
	defer func() { _ = dstFile.Close() }()

	// 1MB buffer 流式写入
	buf := make([]byte, 1024*1024)
	_, err = io.CopyBuffer(dstFile, srcFile, buf)
	if err != nil {
		// 写入失败，删除远程半成品文件
		_ = sftpClient.Remove(remoteFilePath)
		return fmt.Errorf("写入远程文件失败: %w", err)
	}

	// 设置权限
	if err := sftpClient.Chmod(remoteFilePath, perm); err != nil {
		log.Zlog.Warn(fmt.Sprintf("[上传] 设置文件权限失败 - %s: %s: %v", ip, remoteFilePath, err))
	}

	return nil
}

// sftpUploadWithSudo 通过临时目录 + sudo mv 上传文件
func (c *SSHClient) sftpUploadWithSudo(sftpClient *sftp.Client, localPath, fileName, remotePath string, perm os.FileMode, ip string) error {
	// 创建临时目录
	tmpDir := fmt.Sprintf("/tmp/.SSHFleet_tmp/%s", randomHex())
	if err := sftpClient.MkdirAll(tmpDir); err != nil {
		return fmt.Errorf("创建临时目录失败: %w", err)
	}
	log.Zlog.Info(fmt.Sprintf("[上传] 创建临时目录 - %s: %s", ip, tmpDir))

	// 清理函数
	cleanup := func() {
		_ = c.runCommand(fmt.Sprintf("sudo rm -rf %s", tmpDir))
	}

	// 上传到临时目录
	tmpFilePath := tmpDir + "/" + fileName
	srcFile, err := os.Open(localPath)
	if err != nil {
		cleanup()
		return fmt.Errorf("打开本地文件失败: %w", err)
	}
	defer func() { _ = srcFile.Close() }()

	dstFile, err := sftpClient.Create(tmpFilePath)
	if err != nil {
		cleanup()
		return fmt.Errorf("创建临时文件失败: %w", err)
	}
	defer func() { _ = dstFile.Close() }()

	buf := make([]byte, 1024*1024)
	if _, err = io.CopyBuffer(dstFile, srcFile, buf); err != nil {
		cleanup()
		return fmt.Errorf("写入临时文件失败: %w", err)
	}

	// 设置权限
	if err := sftpClient.Chmod(tmpFilePath, perm); err != nil {
		log.Zlog.Warn(fmt.Sprintf("[上传] 设置临时文件权限失败 - %s: %v", ip, err))
	}

	// sudo mv 到目标路径
	mvCmd := fmt.Sprintf("sudo mv %s %s/", tmpFilePath, remotePath)
	if err := c.runCommand(mvCmd); err != nil {
		cleanup()
		return fmt.Errorf("sudo mv 失败: %w", err)
	}
	log.Zlog.Info(fmt.Sprintf("[上传] sudo mv 完成 - %s: %s -> %s/", ip, fileName, remotePath))

	// 清理临时目录
	cleanup()

	return nil
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

// randomHex 生成 8 位随机 hex 字符串
func randomHex() string {
	b := make([]byte, 4)
	rand.Read(b)
	return fmt.Sprintf("%x", b)
}
