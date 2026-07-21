package ssh

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"math/rand"
	"os"
	"strings"
	"time"

	"github.com/pkg/sftp"

	"SSHFleet/internal/log"
	"SSHFleet/internal/localfs"

	"go.uber.org/zap"
)

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
	conn := c.connectSSH()
	result.ConnectCostTime = conn.costTime
	if conn.err != nil {
		result.ConnectSuccess = false
		result.FailedFiles = len(fileItems)
		errMsg := conn.err.Error()
		result.Error = &errMsg
		log.Zlog.Error("[上传] 连接失败", zap.String("ip", ip), zap.Error(conn.err))
		return result, conn.err
	}
	defer func() { _ = c.Close() }()

	result.ConnectSuccess = true
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

	// 6. 预检本地文件
	for _, item := range fileItems {
		if _, err := os.Stat(item.LocalPath); err != nil {
			errMsg := fmt.Sprintf("本地文件不存在或不可读: %s - %v", item.FileName, err)
			result.Error = &errMsg
			result.FailedFiles = len(fileItems)
			log.Zlog.Error("[上传] 本地文件预检失败", zap.String("ip", ip), zap.String("fileName", item.FileName), zap.Error(err))
			return result, fmt.Errorf("%s", errMsg)
		}
	}
	log.Zlog.Debug("[上传] 本地文件预检通过", zap.String("ip", ip), zap.Int("files", len(fileItems)))

	// 7. 判断 sudo 是否实际生效
	effectiveSudo := useSudo && c.config.User != "root"
	if useSudo && c.config.User == "root" {
		log.Zlog.Info("[上传] 用户已是 root，跳过 sudo", zap.String("ip", ip))
	}

	// 8. 逐文件处理
	totalFiles := len(fileItems)
	successFiles := 0
	failedFiles := 0
	var uploadedBytes int64 // 累计已上传字节数
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

		// 8a. 检查远程文件是否已存在
		remoteFilePath := remotePath + "/" + item.FileName
		if _, err := sftpClient.Stat(remoteFilePath); err == nil {
			failedFiles++
			errMsg := fmt.Sprintf("%s: 上传失败 - 文件已存在", item.FileName)
			outputLines = append(outputLines, errMsg)
			result.Error = &errMsg
			log.Zlog.Warn("[上传] 文件已存在，终止传输", zap.String("ip", ip), zap.String("remoteFilePath", remoteFilePath))
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
			break
		}

		// 8b. 读取本地文件权限
		localInfo, err := os.Stat(item.LocalPath)
		if err != nil {
			failedFiles++
			errMsg := fmt.Sprintf("%s: 上传失败 - %v", item.FileName, err)
			outputLines = append(outputLines, errMsg)
			result.Error = &errMsg
			log.Zlog.Error("[上传] 读取本地文件权限失败，终止传输", zap.String("ip", ip), zap.String("fileName", item.FileName), zap.Error(err))
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
			break
		}
		localMode := localInfo.Mode().Perm()

		// 8c. 执行上传（含重试）
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
			errMsg := fmt.Sprintf("%s: 上传失败 - %v", item.FileName, uploadErr)
			outputLines = append(outputLines, errMsg)
			result.Error = &errMsg
			log.Zlog.Error("[上传] 文件上传失败，终止传输", zap.String("ip", ip), zap.String("fileName", item.FileName), zap.Error(uploadErr))
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
			break
		} else {
			successFiles++
			uploadedBytes += fileWritten // 累加已上传字节数
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

	// 9. 构建 output（base64 编码）
	header := fmt.Sprintf("total_files=%d, success_files=%d, failed_files=%d", totalFiles, successFiles, failedFiles)
	outputText := header + "\n" + strings.Join(outputLines, "\n")
	result.Output = base64.StdEncoding.EncodeToString([]byte(outputText))
	// ExitCode: 0=全部成功, 1=有失败
	if failedFiles > 0 {
		result.ExitCode = 1
	} else {
		result.ExitCode = 0
	}
	result.ExecCostTime = totalCostTime
	result.TotalBytes = uploadedBytes // 实际成功上传的字节数
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

// randomHex 生成 8 位随机 hex 字符串
func randomHex() string {
	b := make([]byte, 4)
	rand.Read(b)
	return fmt.Sprintf("%x", b)
}
