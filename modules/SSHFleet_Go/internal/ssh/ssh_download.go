package ssh

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/pkg/sftp"

	"SSHFleet/internal/log"

	"go.uber.org/zap"
)

// DownloadFiles 从远程节点下载文件
func (c *SSHClient) DownloadFiles(
	remotePath string,
	localPath string,
	useSudo bool,
	ctx context.Context,
	seq int,
	ip string,
	onProgress func(ProgressMsg),
) (*UploadResult, error) {
	result := &UploadResult{
		Type: "result",
		IP:   c.config.IP,
		Port: c.config.Port,
		User: c.config.User,
	}

	// 1. SSH 建连
	conn := c.connectSSH()
	result.ConnectCostTime = conn.costTime
	if conn.err != nil {
		result.ConnectSuccess = false
		errMsg := conn.err.Error()
		result.Error = &errMsg
		log.Zlog.Error("[下载] 连接失败", zap.String("ip", ip), zap.Error(conn.err))
		return result, conn.err
	}
	defer func() { _ = c.Close() }()

	result.ConnectSuccess = true
	log.Zlog.Succ("[下载] 连接成功", zap.String("ip", ip))

	// 2. 创建 SFTP 客户端
	sftpClient, err := sftp.NewClient(c.client)
	if err != nil {
		errMsg := err.Error()
		result.Error = &errMsg
		log.Zlog.Error("[下载] SFTP 客户端创建失败", zap.String("ip", ip), zap.Error(err))
		return result, err
	}
	defer func() { _ = sftpClient.Close() }()
	log.Zlog.Debug("[下载] SFTP 客户端创建成功", zap.String("ip", ip))

	// 3. 预检查远程路径是否存在（可选 sudo）
	effectiveSudo := useSudo && c.config.User != "root"
	checkCmd := fmt.Sprintf("test -e '%s'", remotePath)
	if effectiveSudo {
		checkCmd = fmt.Sprintf("sudo test -e '%s'", remotePath)
	}
	if err := c.runCommand(checkCmd); err != nil {
		errMsg := fmt.Sprintf("远程路径不存在: %s", remotePath)
		result.Error = &errMsg
		log.Zlog.Error("[下载] 远程路径不存在", zap.String("ip", ip), zap.String("remotePath", remotePath))
		return result, fmt.Errorf("%s", errMsg)
	}

	// 4. SFTP Stat 判断文件/目录
	fi, err := sftpClient.Stat(remotePath)
	if err != nil {
		errMsg := fmt.Sprintf("远程路径不可访问: %s", remotePath)
		result.Error = &errMsg
		log.Zlog.Error("[下载] 远程路径不可访问", zap.String("ip", ip), zap.Error(err))
		return result, fmt.Errorf("%s", errMsg)
	}

	// 5. 预计算总字节数和文件列表（目录模式用命令获取）
	type remoteFile struct {
		relativePath string
		size         int64
	}
	var files []remoteFile
	var totalBytes int64

	if fi.IsDir() {
		// 目录模式：用 find 命令获取文件列表和大小
		findCmd := fmt.Sprintf("find '%s' -type f -printf '%%s %%P\\n'", remotePath)
		if effectiveSudo {
			findCmd = fmt.Sprintf("sudo find '%s' -type f -printf '%%s %%P\\n'", remotePath)
		}
		output, err := c.sftpRunCommand(findCmd)
		if err != nil {
			errMsg := fmt.Sprintf("获取远程文件列表失败: %v", err)
			result.Error = &errMsg
			log.Zlog.Error("[下载] 获取远程文件列表失败", zap.String("ip", ip), zap.Error(err))
			return result, fmt.Errorf("%s", errMsg)
		}
		output = strings.TrimSpace(output)
		if output == "" {
			errMsg := "远程目录为空"
			result.Error = &errMsg
			log.Zlog.Error("[下载] 远程目录为空", zap.String("ip", ip), zap.String("remotePath", remotePath))
			return result, fmt.Errorf("%s", errMsg)
		}
		for _, line := range strings.Split(output, "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			parts := strings.SplitN(line, " ", 2)
			if len(parts) != 2 {
				continue
			}
			var size int64
			fmt.Sscanf(parts[0], "%d", &size)
			files = append(files, remoteFile{relativePath: parts[1], size: size})
			totalBytes += size
		}
	} else {
		// 文件模式
		files = append(files, remoteFile{relativePath: filepath.Base(remotePath), size: fi.Size()})
		totalBytes = fi.Size()
	}

	totalFiles := len(files)
	if totalFiles == 0 {
		errMsg := "远程路径中没有可下载的文件"
		result.Error = &errMsg
		log.Zlog.Error("[下载] 没有可下载的文件", zap.String("ip", ip))
		return result, fmt.Errorf("%s", errMsg)
	}

	log.Zlog.Info("[下载] 文件清单", zap.String("ip", ip), zap.Int("totalFiles", totalFiles), zap.Int64("totalBytes", totalBytes))

	// 6. 创建本地目录
	ipDir := filepath.Join(localPath, ip)
	if err := os.MkdirAll(ipDir, 0755); err != nil {
		errMsg := fmt.Sprintf("创建本地目录失败: %v", err)
		result.Error = &errMsg
		log.Zlog.Error("[下载] 创建本地目录失败", zap.String("ip", ip), zap.String("ipDir", ipDir), zap.Error(err))
		return result, fmt.Errorf("%s", errMsg)
	}

	// 7. 逐文件下载
	successFiles := 0
	failedFiles := 0
	var downloadedBytes int64
	var outputLines []string
	totalCostTime := 0.0

	for _, file := range files {
		select {
		case <-ctx.Done():
			errMsg := "下载被取消"
			result.Error = &errMsg
			return result, ctx.Err()
		default:
		}

		fileStart := time.Now()

		// 构建远程和本地路径
		var remoteFilePath, localFilePath string
		if fi.IsDir() {
			remoteFilePath = remotePath + "/" + file.relativePath
			localFilePath = filepath.Join(ipDir, file.relativePath)
		} else {
			remoteFilePath = remotePath
			localFilePath = filepath.Join(ipDir, filepath.Base(remotePath))
		}

		// 确保本地子目录存在
		localDir := filepath.Dir(localFilePath)
		if err := os.MkdirAll(localDir, 0755); err != nil {
			failedFiles++
			errMsg := fmt.Sprintf("%s: 下载失败 - 创建本地目录失败: %v", file.relativePath, err)
			outputLines = append(outputLines, errMsg)
			result.Error = &errMsg
			log.Zlog.Error("[下载] 创建本地子目录失败", zap.String("ip", ip), zap.String("localDir", localDir), zap.Error(err))
			if onProgress != nil {
				onProgress(ProgressMsg{
					Type:            "progress",
					Seq:             seq,
					IP:              ip,
					DownloadedBytes: downloadedBytes,
					TotalBytes:      totalBytes,
					TotalFiles:      totalFiles,
					SuccessFiles:    successFiles,
					FailedFiles:     failedFiles,
				})
			}
			break
		}

		// 下载文件
		var downloadErr error
		var fileWritten int64
		for attempt := 0; attempt <= maxFileRetries; attempt++ {
			fileWritten, downloadErr = c.sftpDownloadFile(sftpClient, remoteFilePath, localFilePath, seq, ip, totalBytes, totalFiles, onProgress)
			if downloadErr == nil {
				break
			}
			if attempt < maxFileRetries {
				log.Zlog.Warn("[下载] 文件下载失败，准备重试",
					zap.String("ip", ip), zap.String("remoteFilePath", remoteFilePath),
					zap.Int("attempt", attempt+1), zap.Int("maxRetries", maxFileRetries),
					zap.Error(downloadErr))
				time.Sleep(retryInterval)
			}
		}

		costTime := time.Since(fileStart).Seconds()
		totalCostTime += costTime

		if downloadErr != nil {
			failedFiles++
			errMsg := fmt.Sprintf("%s: 下载失败 - %v", file.relativePath, downloadErr)
			outputLines = append(outputLines, errMsg)
			result.Error = &errMsg
			log.Zlog.Error("[下载] 文件下载失败", zap.String("ip", ip), zap.String("remoteFilePath", remoteFilePath), zap.Error(downloadErr))
			// 删除半成品文件
			_ = os.Remove(localFilePath)
			if onProgress != nil {
				onProgress(ProgressMsg{
					Type:            "progress",
					Seq:             seq,
					IP:              ip,
					DownloadedBytes: downloadedBytes,
					TotalBytes:      totalBytes,
					TotalFiles:      totalFiles,
					SuccessFiles:    successFiles,
					FailedFiles:     failedFiles,
				})
			}
			break
		} else {
			successFiles++
			downloadedBytes += fileWritten
			outputLines = append(outputLines, fmt.Sprintf("%s: 下载成功 (%.3fs)", file.relativePath, costTime))
			log.Zlog.Debug("[下载] 文件下载成功", zap.String("ip", ip), zap.String("remoteFilePath", remoteFilePath), zap.Float64("costTime", costTime))
		}

		// 发送进度更新
		if onProgress != nil {
			msg := ProgressMsg{
				Type:            "progress",
				Seq:             seq,
				IP:              ip,
				DownloadedBytes: downloadedBytes,
				TotalBytes:      totalBytes,
				TotalFiles:      totalFiles,
				SuccessFiles:    successFiles,
				FailedFiles:     failedFiles,
			}
			log.Zlog.Info("[下载] 发送进度", zap.Int("seq", seq), zap.Int64("downloadedBytes", downloadedBytes), zap.Int("success", successFiles), zap.Int("failed", failedFiles))
			onProgress(msg)
		}
	}

	// 8. 构建 output（base64 编码）
	header := fmt.Sprintf("total_files=%d, success_files=%d, failed_files=%d", totalFiles, successFiles, failedFiles)
	outputText := header + "\n" + strings.Join(outputLines, "\n")
	result.Output = base64.StdEncoding.EncodeToString([]byte(outputText))
	if failedFiles > 0 {
		code := 1
		result.ExitCode = &code
	} else {
		code := 0
		result.ExitCode = &code
	}
	result.ExecCostTime = totalCostTime
	result.TotalBytes = downloadedBytes
	result.TotalFiles = totalFiles
	result.SuccessFiles = successFiles
	result.FailedFiles = failedFiles

	log.Zlog.Succ("[下载] 节点完成", zap.String("ip", ip), zap.Int("success", successFiles), zap.Int("total", totalFiles))
	return result, nil
}

// sftpDownloadFile 通过 SFTP 下载单个文件，返回实际下载的字节数
func (c *SSHClient) sftpDownloadFile(sftpClient *sftp.Client, remoteFilePath, localFilePath string, seq int, ip string, totalBytes int64, totalFiles int, onProgress func(ProgressMsg)) (int64, error) {
	// 检查远程是否为符号链接（跳过符号链接）
	if fi, err := sftpClient.Lstat(remoteFilePath); err == nil && fi.Mode()&os.ModeSymlink != 0 {
		return 0, fmt.Errorf("跳过符号链接: %s", remoteFilePath)
	}

	srcFile, err := sftpClient.Open(remoteFilePath)
	if err != nil {
		return 0, fmt.Errorf("打开远程文件失败: %w", err)
	}
	defer func() { _ = srcFile.Close() }()

	dstFile, err := os.Create(localFilePath)
	if err != nil {
		return 0, fmt.Errorf("创建本地文件失败: %w", err)
	}
	defer func() { _ = dstFile.Close() }()

	// 1MB buffer 流式写入
	buf := make([]byte, 1024*1024)
	var written int64
	if onProgress != nil {
		pr := &progressReader{
			src:        srcFile,
			seq:        seq,
			ip:         ip,
			totalBytes: totalBytes,
			totalFiles: totalFiles,
			callback:   onProgress,
		}
		written, err = io.CopyBuffer(dstFile, pr, buf)
	} else {
		written, err = io.CopyBuffer(dstFile, srcFile, buf)
	}
	if err != nil {
		// 写入失败，删除本地半成品文件
		_ = os.Remove(localFilePath)
		return 0, fmt.Errorf("写入本地文件失败: %w", err)
	}

	return written, nil
}

// sftpRunCommand 通过 SSH 执行命令并返回输出（用于预检查）
func (c *SSHClient) sftpRunCommand(command string) (string, error) {
	session, err := c.client.NewSession()
	if err != nil {
		return "", err
	}
	defer func() { _ = session.Close() }()

	output, err := session.CombinedOutput(command)
	return string(output), err
}
