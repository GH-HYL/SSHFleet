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
	"strings"
	"sync"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"

	"SSHFleet/internal/log"
	"SSHFleet/internal/localfs"

	"go.uber.org/zap"
)

// SSHClient 封装SSH客户端连接
type SSHClient struct {
	client *ssh.Client
	config *SSHConfig
}

const (
	maxFileRetries = 2
	retryInterval  = 2 * time.Second
	progressThrottle = 500 * time.Millisecond
)

// threadSafeWriter 带锁的写入器，确保并发写入安全且保持顺序
type threadSafeWriter struct {
	buf *bytes.Buffer
	mu  sync.Mutex
}

func (w *threadSafeWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.Write(p)
}

// progressWriter 带进度回调的写入器
type progressWriter struct {
	dst          io.Writer
	uploaded     int64
	lastCallback time.Time
	seq          int
	ip           string
	totalBytes   int64
	totalFiles   int
	callback     func(ProgressMsg)
	mu           sync.Mutex
}

func (pw *progressWriter) Write(p []byte) (int, error) {
	n, err := pw.dst.Write(p)
	pw.mu.Lock()
	pw.uploaded += int64(n)
	if time.Since(pw.lastCallback) >= progressThrottle {
		pw.lastCallback = time.Now()
		pw.callback(ProgressMsg{
			Type:          "progress",
			Seq:           pw.seq,
			IP:            pw.ip,
			UploadedBytes: pw.uploaded,
			TotalBytes:    pw.totalBytes,
			TotalFiles:    pw.totalFiles,
		})
	}
	pw.mu.Unlock()
	return n, err
}

// ProgressMsg SSE 进度消息
type ProgressMsg struct {
	Type          string `json:"type"`
	Seq           int    `json:"seq"`
	IP            string `json:"ip"`
	UploadedBytes int64  `json:"uploaded_bytes,omitempty"`
	TotalBytes    int64  `json:"total_bytes,omitempty"`
	TotalFiles    int    `json:"total_files,omitempty"`
	SuccessFiles  int    `json:"success_files,omitempty"`
	FailedFiles   int    `json:"failed_files,omitempty"`
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
	addr := fmt.Sprintf("%s:%d", c.config.IP, c.config.Port)
	log.Zlog.Debug("SSH连接", zap.String("addr", addr))

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
		client, err := ssh.Dial("tcp", addr, sshClientConfig)
		result <- dialResult{client, err}
	}()

	maxTimeout := c.config.ConnectTimeout + 2*time.Second
	select {
	case res := <-result:
		if res.err != nil {
			log.Zlog.Error("SSH连接失败", zap.String("user", c.config.User), zap.String("addr", addr), zap.Error(res.err))
			return fmt.Errorf("建立连接失败 - %w", res.err)
		}
		c.client = res.client
		log.Zlog.Succ("SSH连接成功", zap.String("user", c.config.User), zap.String("addr", addr))
		return nil
	case <-time.After(maxTimeout):
		log.Zlog.Error("SSH连接失败: 握手超时", zap.String("user", c.config.User), zap.String("addr", addr), zap.Duration("timeout", maxTimeout))
		return fmt.Errorf("建立连接失败 - 握手超时%v", maxTimeout)
	}
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
	connStart := time.Now()
	if err := c.Connect(); err != nil {
		result.ConnectCostTime = time.Since(connStart).Seconds()
		result.ConnectSuccess = false
		result.ExitCode = -1
		errMsg := err.Error()
		result.Error = &errMsg
		log.Zlog.Error("连接失败", zap.String("ip", ip), zap.Error(err))
		return result, err
	}
	defer func() { _ = c.Close() }()

	result.ConnectSuccess = true
	result.ConnectCostTime = time.Since(connStart).Seconds()
	log.Zlog.Succ("连接成功", zap.String("ip", ip))

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
		log.Zlog.Warn("命令执行超时", zap.Duration("timeout", c.config.ExecTimeout))
		return fmt.Errorf("命令执行超时(%v)", c.config.ExecTimeout)
	}
}

// UploadFiles 上传文件到远程节点
func (c *SSHClient) UploadFiles(
	fileItems []localfs.FileItem,
	remotePath string,
	useSudo bool,
	ctx context.Context,
	seq int,
	ip string,
	onProgress func(ProgressMsg),
) (*UploadResult, error) {
	result := &UploadResult{
		Type:     "result",
		IP:       c.config.IP,
		Port:     c.config.Port,
		User:     c.config.User,
		ExitCode: 0,
	}

	// 1. SSH 建连
	connStart := time.Now()
	if err := c.Connect(); err != nil {
		result.ConnectCostTime = time.Since(connStart).Seconds()
		result.ConnectSuccess = false
		result.FailedFiles = len(fileItems)
		errMsg := err.Error()
		result.Error = &errMsg
		log.Zlog.Error("[上传] 连接失败", zap.String("ip", ip), zap.Error(err))
		return result, err
	}
	defer func() { _ = c.Close() }()

	result.ConnectSuccess = true
	result.ConnectCostTime = time.Since(connStart).Seconds()
	log.Zlog.Succ("[上传] 连接成功", zap.String("ip", ip))

	// 2. 清理残留临时目录（仅 sudo 模式）
	if useSudo {
		c.runCommand("sudo rm -rf /tmp/.SSHFleet_tmp/")
		log.Zlog.Debug("[上传] 清理残留临时目录", zap.String("ip", ip))
	}

	// 3. 创建 SFTP 客户端
	sftpClient, err := sftp.NewClient(c.client)
	if err != nil {
		errMsg := err.Error()
		result.Error = &errMsg
		log.Zlog.Error("[上传] SFTP 客户端创建失败", zap.String("ip", ip), zap.Error(err))
		return result, err
	}
	defer func() { _ = sftpClient.Close() }()
	log.Zlog.Debug("[上传] SFTP 客户端创建成功", zap.String("ip", ip))

	// 4. 计算总字节数并发送首次进度（必须在路径检查之前，确保 Python 能创建进度条）
	var totalBytes int64
	for _, item := range fileItems {
		totalBytes += item.FileSize
	}
	if onProgress != nil {
		log.Zlog.Info("[上传] 发送首次进度", zap.Int("seq", seq), zap.Int64("totalBytes", totalBytes), zap.Int("totalFiles", len(fileItems)))
		onProgress(ProgressMsg{
			Type:       "progress",
			Seq:        seq,
			IP:         ip,
			TotalBytes: totalBytes,
			TotalFiles: len(fileItems),
		})
	}

	// 5. 检查远程目标路径
	fi, err := sftpClient.Stat(remotePath)
	if err != nil {
		errMsg := fmt.Sprintf("远程目标路径不存在: %s", remotePath)
		result.Error = &errMsg
		log.Zlog.Error("[上传] 远程目标路径不存在", zap.String("ip", ip), zap.String("remotePath", remotePath))
		return result, fmt.Errorf("%s", errMsg)
	}
	if !fi.IsDir() {
		errMsg := fmt.Sprintf("远程目标路径不是目录: %s", remotePath)
		result.Error = &errMsg
		log.Zlog.Error("[上传] 远程目标路径不是目录", zap.String("ip", ip), zap.String("remotePath", remotePath))
		return result, fmt.Errorf("%s", errMsg)
	}
	log.Zlog.Debug("[上传] 远程目标路径检查通过", zap.String("ip", ip), zap.String("remotePath", remotePath))

	// 6. 判断 sudo 是否实际生效
	effectiveSudo := useSudo && c.config.User != "root"
	if useSudo && c.config.User == "root" {
		log.Zlog.Info("[上传] 用户已是 root，跳过 sudo", zap.String("ip", ip))
	}

	// 7. 逐文件处理
	totalFiles := len(fileItems)
	successFiles := 0
	failedFiles := 0
	var uploadedBytes int64  // 累计已上传字节数
	var outputLines []string
	totalCostTime := 0.0

	for _, item := range fileItems {
		select {
		case <-ctx.Done():
			errMsg := "上传被取消"
			result.Error = &errMsg
			return result, ctx.Err()
		default:
		}

		fileStart := time.Now()

		// 7a. 检查远程文件是否已存在
		remoteFilePath := remotePath + "/" + item.FileName
		if _, err := sftpClient.Stat(remoteFilePath); err == nil {
			failedFiles++
			outputLines = append(outputLines, fmt.Sprintf("%s: 上传失败 - 文件已存在", item.FileName))
			log.Zlog.Warn("[上传] 文件已存在，跳过", zap.String("ip", ip), zap.String("remoteFilePath", remoteFilePath))
			// 文件完成：发送进度更新
			if onProgress != nil {
				onProgress(ProgressMsg{
					Type:         "progress",
					Seq:          seq,
					IP:           ip,
					SuccessFiles: successFiles,
					FailedFiles:  failedFiles,
				})
			}
			continue
		}

		// 7b. 读取本地文件权限
		localInfo, err := os.Stat(item.LocalPath)
		if err != nil {
			failedFiles++
			outputLines = append(outputLines, fmt.Sprintf("%s: 上传失败 - %v", item.FileName, err))
			log.Zlog.Error("[上传] 读取本地文件权限失败", zap.String("ip", ip), zap.String("fileName", item.FileName), zap.Error(err))
			// 文件完成：发送进度更新
			if onProgress != nil {
				onProgress(ProgressMsg{
					Type:         "progress",
					Seq:          seq,
					IP:           ip,
					SuccessFiles: successFiles,
					FailedFiles:  failedFiles,
				})
			}
			continue
		}
		localMode := localInfo.Mode().Perm()

		// 7c. 执行上传（含重试）
		var uploadErr error
		var fileWritten int64
		for attempt := 0; attempt <= maxFileRetries; attempt++ {
			if !effectiveSudo {
				fileWritten, uploadErr = c.sftpUploadFile(sftpClient, item.LocalPath, remoteFilePath, localMode, seq, ip, totalBytes, totalFiles, onProgress)
			} else {
				fileWritten, uploadErr = c.sftpUploadWithSudo(sftpClient, item.LocalPath, item.FileName, remotePath, localMode, seq, ip, totalBytes, totalFiles, onProgress)
			}
			if uploadErr == nil {
				break
			}
			if attempt < maxFileRetries {
				log.Zlog.Warn("[上传] 文件上传失败，准备重试",
					zap.String("ip", ip), zap.String("fileName", item.FileName),
					zap.Int("attempt", attempt+1), zap.Int("maxRetries", maxFileRetries),
					zap.Error(uploadErr))
				time.Sleep(retryInterval)
			}
		}

		costTime := time.Since(fileStart).Seconds()
		totalCostTime += costTime

		if uploadErr != nil {
			failedFiles++
			outputLines = append(outputLines, fmt.Sprintf("%s: 上传失败 - %v", item.FileName, uploadErr))
			log.Zlog.Error("[上传] 文件上传失败", zap.String("ip", ip), zap.String("fileName", item.FileName), zap.Error(uploadErr))
		} else {
			successFiles++
			uploadedBytes += fileWritten  // 累加已上传字节数
			outputLines = append(outputLines, fmt.Sprintf("%s: 上传成功 (%.3fs)", item.FileName, costTime))
			log.Zlog.Debug("[上传] 文件上传成功", zap.String("ip", ip), zap.String("fileName", item.FileName), zap.Float64("costTime", costTime))
		}

		// 文件完成：发送进度更新
		if onProgress != nil {
			msg := ProgressMsg{
				Type:          "progress",
				Seq:           seq,
				IP:            ip,
				UploadedBytes: uploadedBytes,
				TotalBytes:    totalBytes,
				TotalFiles:    totalFiles,
				SuccessFiles:  successFiles,
				FailedFiles:   failedFiles,
			}
			log.Zlog.Info("[上传] 发送进度", zap.Int("seq", seq), zap.Int64("uploadedBytes", uploadedBytes), zap.Int("success", successFiles), zap.Int("failed", failedFiles))
			onProgress(msg)
		}
	}

	// 8. 构建 output（base64 编码）
	header := fmt.Sprintf("total_files=%d, success_files=%d, failed_files=%d", totalFiles, successFiles, failedFiles)
	outputText := header + "\n" + strings.Join(outputLines, "\n")
	result.Output = base64.StdEncoding.EncodeToString([]byte(outputText))
	result.ExitCode = failedFiles
	result.ExecCostTime = totalCostTime
	result.TotalBytes = uploadedBytes  // 使用实际上传字节数，而不是总字节数
	result.TotalFiles = totalFiles
	result.SuccessFiles = successFiles
	result.FailedFiles = failedFiles

	log.Zlog.Succ("[上传] 节点完成", zap.String("ip", ip), zap.Int("success", successFiles), zap.Int("total", totalFiles))
	return result, nil
}

// sftpUploadFile 直接通过 SFTP 写入文件，返回实际上传的字节数
func (c *SSHClient) sftpUploadFile(sftpClient *sftp.Client, localPath, remoteFilePath string, perm os.FileMode, seq int, ip string, totalBytes int64, totalFiles int, onProgress func(ProgressMsg)) (int64, error) {
	srcFile, err := os.Open(localPath)
	if err != nil {
		return 0, fmt.Errorf("打开本地文件失败: %w", err)
	}
	defer func() { _ = srcFile.Close() }()

	dstFile, err := sftpClient.Create(remoteFilePath)
	if err != nil {
		return 0, fmt.Errorf("创建远程文件失败: %w", err)
	}
	defer func() { _ = dstFile.Close() }()

	// 1MB buffer 流式写入
	buf := make([]byte, 1024*1024)
	var written int64
	if onProgress != nil {
		pw := &progressWriter{
			dst:        dstFile,
			seq:        seq,
			ip:         ip,
			totalBytes: totalBytes,
			totalFiles: totalFiles,
			callback:   onProgress,
		}
		written, err = io.CopyBuffer(pw, srcFile, buf)
	} else {
		written, err = io.CopyBuffer(dstFile, srcFile, buf)
	}
	if err != nil {
		// 写入失败，删除远程半成品文件
		_ = sftpClient.Remove(remoteFilePath)
		return 0, fmt.Errorf("写入远程文件失败: %w", err)
	}

	// 设置权限
	if err := sftpClient.Chmod(remoteFilePath, perm); err != nil {
		log.Zlog.Warn("[上传] 设置文件权限失败", zap.String("ip", ip), zap.String("remoteFilePath", remoteFilePath), zap.Error(err))
	}

	return written, nil
}

// sftpUploadWithSudo 通过临时目录 + sudo mv 上传文件，返回实际上传的字节数
func (c *SSHClient) sftpUploadWithSudo(sftpClient *sftp.Client, localPath, fileName, remotePath string, perm os.FileMode, seq int, ip string, totalBytes int64, totalFiles int, onProgress func(ProgressMsg)) (int64, error) {
	// 创建临时目录（使用 SSH + sudo，因为 SFTP 没有 sudo 权限）
	tmpDir := fmt.Sprintf("/tmp/.SSHFleet_tmp/%s", randomHex())
	if err := c.runCommand(fmt.Sprintf("sudo mkdir -p '%s' && sudo chmod 777 '%s'", tmpDir, tmpDir)); err != nil {
		return 0, fmt.Errorf("创建临时目录失败: %w", err)
	}
	log.Zlog.Debug("[上传] 创建临时目录", zap.String("ip", ip), zap.String("tmpDir", tmpDir))

	// 清理函数
	cleanup := func() {
		_ = c.runCommand(fmt.Sprintf("sudo rm -rf '%s'", tmpDir))
	}

	// 上传到临时目录
	tmpFilePath := tmpDir + "/" + fileName
	srcFile, err := os.Open(localPath)
	if err != nil {
		cleanup()
		return 0, fmt.Errorf("打开本地文件失败: %w", err)
	}
	defer func() { _ = srcFile.Close() }()

	dstFile, err := sftpClient.Create(tmpFilePath)
	if err != nil {
		cleanup()
		return 0, fmt.Errorf("创建临时文件失败: %w", err)
	}
	defer func() { _ = dstFile.Close() }()

	buf := make([]byte, 1024*1024)
	var written int64
	if onProgress != nil {
		pw := &progressWriter{
			dst:        dstFile,
			seq:        seq,
			ip:         ip,
			totalBytes: totalBytes,
			totalFiles: totalFiles,
			callback:   onProgress,
		}
		written, err = io.CopyBuffer(pw, srcFile, buf)
		if err != nil {
			cleanup()
			return 0, fmt.Errorf("写入临时文件失败: %w", err)
		}
	} else {
		written, err = io.CopyBuffer(dstFile, srcFile, buf)
		if err != nil {
			cleanup()
			return 0, fmt.Errorf("写入临时文件失败: %w", err)
		}
	}

	// 设置权限
	if err := sftpClient.Chmod(tmpFilePath, perm); err != nil {
		log.Zlog.Warn("[上传] 设置临时文件权限失败", zap.String("ip", ip), zap.Error(err))
	}

	// sudo mv 到目标路径（引号防止路径含特殊字符导致命令注入）
	escapedRemotePath := strings.ReplaceAll(remotePath, "'", "'\\''")
	mvCmd := fmt.Sprintf("sudo mv '%s' '%s/'", tmpFilePath, escapedRemotePath)
	if err := c.runCommand(mvCmd); err != nil {
		cleanup()
		return 0, fmt.Errorf("sudo mv 失败: %w", err)
	}
	log.Zlog.Info("[上传] sudo mv 完成", zap.String("ip", ip), zap.String("fileName", fileName), zap.String("remotePath", remotePath))

	// 清理临时目录
	cleanup()

	return written, nil
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
