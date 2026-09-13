package ssh

import (
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/pkg/sftp"
)

// tmpRoot sudo 模式下的远端临时目录根（旧实现同名，上传前会整体清理）。
const tmpRoot = "/tmp/.SSHFleet_tmp/"

// UploadFiles 上传本地文件清单到远程目录（对位旧 UploadFiles）。
// 前置：远程目标目录**必须已存在**（用户 2026-09-11 裁定 Q7：工具不建目录，错路径当场报错）。
func (c *Client) UploadFiles(ctx context.Context, files []LocalFile, remotePath string, useSudo bool, seq int, onProgress func(Progress)) *Result {
	result := &Result{
		Seq:        seq,
		IP:         c.cfg.IP,
		Port:       c.cfg.Port,
		User:       c.cfg.User,
		TotalFiles: len(files),
	}

	start := time.Now()
	if err := c.Connect(ctx); err != nil {
		result.ConnectCostTime = time.Since(start).Seconds()
		result.FailedFiles = len(files)
		result.Error = strPtr(err.Error())
		result.AuthFailure = c.classifyAuthFailure(err)
		return result
	}
	defer func() { _ = c.Close() }()
	result.ConnectCostTime = time.Since(start).Seconds()
	result.ConnectSuccess = true

	// 清理残留临时目录（仅 sudo 模式）
	if useSudo {
		_ = c.runCommand("sudo rm -rf " + tmpRoot)
	}

	sftpClient, err := sftp.NewClient(c.conn)
	if err != nil {
		result.Error = strPtr("SFTP 客户端创建失败 - " + err.Error())
		return result
	}
	defer func() { _ = sftpClient.Close() }()

	var totalBytes int64
	for _, f := range files {
		totalBytes += f.Size
	}
	// 首发进度：必须在路径检查之前，便于上层立刻建立该节点进度
	if onProgress != nil {
		onProgress(Progress{Seq: seq, IP: c.cfg.IP, TotalBytes: totalBytes, TotalFiles: len(files)})
	}

	// 远程目标路径检查：必须存在且是目录
	fi, err := sftpClient.Stat(remotePath)
	if err != nil {
		result.Error = strPtr(fmt.Sprintf("远程目标路径不存在: %s", remotePath))
		return result
	}
	if !fi.IsDir() {
		result.Error = strPtr(fmt.Sprintf("远程目标路径不是目录: %s", remotePath))
		return result
	}

	// 本地文件预检
	for _, f := range files {
		if _, err := os.Stat(f.Path); err != nil {
			result.FailedFiles = len(files)
			result.Error = strPtr(fmt.Sprintf("本地文件不存在或不可读: %s - %v", f.Name, err))
			return result
		}
	}

	// sudo 生效判断：root 用户无需 sudo（旧行为）
	effectiveSudo := useSudo && c.cfg.User != "root"

	success, failed := 0, 0
	var uploadedBytes int64
	var lines []string
	var costTotal float64
	pacer := &progressPacer{}

	for _, f := range files {
		select {
		case <-ctx.Done():
			result.Error = strPtr("上传被取消")
			result.SuccessFiles, result.FailedFiles = success, failed
			result.TotalBytes, result.ExecCostTime = uploadedBytes, costTotal
			result.Output = buildTransferOutput(len(files), success, failed, lines)
			return result
		default:
		}

		fileStart := time.Now()
		remoteFilePath := remotePath + "/" + f.Name

		// 远程文件已存在：该文件失败并终止本节点传输（不覆盖，旧行为）
		if _, err := sftpClient.Stat(remoteFilePath); err == nil {
			failed++
			msg := fmt.Sprintf("%s: 上传失败 - 文件已存在", f.Name)
			lines = append(lines, msg)
			result.Error = strPtr(msg)
			if onProgress != nil {
				onProgress(Progress{Seq: seq, IP: c.cfg.IP, SuccessFiles: success, FailedFiles: failed})
			}
			break
		}

		localInfo, err := os.Stat(f.Path)
		if err != nil {
			failed++
			msg := fmt.Sprintf("%s: 上传失败 - %v", f.Name, err)
			lines = append(lines, msg)
			result.Error = strPtr(msg)
			if onProgress != nil {
				onProgress(Progress{Seq: seq, IP: c.cfg.IP, SuccessFiles: success, FailedFiles: failed})
			}
			break
		}
		localMode := localInfo.Mode().Perm()

		var (
			written   int64
			uploadErr error
		)
		if !effectiveSudo {
			written, uploadErr = c.sftpUploadFile(sftpClient, f.Path, remoteFilePath, localMode, seq, totalBytes, len(files), onProgress)
		} else {
			written, uploadErr = c.sftpUploadWithSudo(sftpClient, f.Path, f.Name, remotePath, localMode, seq, totalBytes, len(files), onProgress)
		}

		cost := time.Since(fileStart).Seconds()
		costTotal += cost

		if uploadErr != nil {
			failed++
			msg := fmt.Sprintf("%s: 上传失败 - %v", f.Name, uploadErr)
			lines = append(lines, msg)
			result.Error = strPtr(msg)
			if onProgress != nil {
				onProgress(Progress{Seq: seq, IP: c.cfg.IP, SuccessFiles: success, FailedFiles: failed})
			}
			break
		}
		success++
		uploadedBytes += written
		lines = append(lines, fmt.Sprintf("%s: 上传成功 (%.3fs)", f.Name, cost))

		if onProgress != nil && pacer.allow() {
			onProgress(Progress{Seq: seq, IP: c.cfg.IP, UploadedBytes: uploadedBytes, TotalBytes: totalBytes, TotalFiles: len(files), SuccessFiles: success, FailedFiles: failed})
		}
	}

	result.Output = buildTransferOutput(len(files), success, failed, lines)
	// 退出码语义：传输阶段不是命令执行，失败时不设退出码；仅全部成功置 0
	if failed == 0 {
		result.ExitCode = intPtr(0)
	}
	result.ExecCostTime = costTotal
	result.TotalBytes = uploadedBytes
	result.TotalFiles = len(files)
	result.SuccessFiles = success
	result.FailedFiles = failed
	return result
}

// DownloadFiles 从远程路径下载文件到本地目录（对位旧 DownloadFiles）：
// 目录模式下按 IP 建子目录、保留远程相对路径；符号链接跳过不计失败。
func (c *Client) DownloadFiles(ctx context.Context, remotePath, localPath string, useSudo bool, seq int, onProgress func(Progress)) *Result {
	result := &Result{
		Seq:  seq,
		IP:   c.cfg.IP,
		Port: c.cfg.Port,
		User: c.cfg.User,
	}

	start := time.Now()
	if err := c.Connect(ctx); err != nil {
		result.ConnectCostTime = time.Since(start).Seconds()
		result.Error = strPtr(err.Error())
		result.AuthFailure = c.classifyAuthFailure(err)
		return result
	}
	defer func() { _ = c.Close() }()
	result.ConnectCostTime = time.Since(start).Seconds()
	result.ConnectSuccess = true

	sftpClient, err := sftp.NewClient(c.conn)
	if err != nil {
		result.Error = strPtr("SFTP 客户端创建失败 - " + err.Error())
		return result
	}
	defer func() { _ = sftpClient.Close() }()

	effectiveSudo := useSudo && c.cfg.User != "root"

	// 远程路径预检（sudo 时用 sudo test）
	checkCmd := fmt.Sprintf("test -e '%s'", remotePath)
	if effectiveSudo {
		checkCmd = fmt.Sprintf("sudo test -e '%s'", remotePath)
	}
	if err := c.runCommand(checkCmd); err != nil {
		result.ExitCode = extractExitCode(err)
		result.Error = strPtr(fmt.Sprintf("远程路径不存在: %s", remotePath))
		return result
	}

	fi, err := sftpClient.Stat(remotePath)
	if err != nil {
		result.Error = strPtr(fmt.Sprintf("远程路径不可访问: %s", remotePath))
		return result
	}

	type remoteFile struct {
		relativePath string
		size         int64
	}
	var (
		files      []remoteFile
		totalBytes int64
	)
	if fi.IsDir() {
		findCmd := fmt.Sprintf("find '%s' -type f -printf '%%s %%P\\n'", remotePath)
		if effectiveSudo {
			findCmd = fmt.Sprintf("sudo find '%s' -type f -printf '%%s %%P\\n'", remotePath)
		}
		output, err := c.runCommandCapture(findCmd)
		if err != nil {
			result.ExitCode = extractExitCode(err)
			result.Error = strPtr(fmt.Sprintf("获取远程文件列表失败: %v", err))
			return result
		}
		output = strings.TrimSpace(output)
		if output == "" {
			result.Error = strPtr("远程目录为空")
			return result
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
			_, _ = fmt.Sscanf(parts[0], "%d", &size)
			files = append(files, remoteFile{relativePath: parts[1], size: size})
			totalBytes += size
		}
	} else {
		files = append(files, remoteFile{relativePath: filepath.Base(remotePath), size: fi.Size()})
		totalBytes = fi.Size()
	}

	totalFiles := len(files)
	if totalFiles == 0 {
		result.Error = strPtr("远程路径中没有可下载的文件")
		return result
	}

	ipDir := filepath.Join(localPath, c.cfg.IP)
	if err := os.MkdirAll(ipDir, 0o755); err != nil {
		result.Error = strPtr(fmt.Sprintf("创建本地目录失败: %v", err))
		return result
	}

	success, failed, skipped := 0, 0, 0
	var downloadedBytes int64
	var lines []string
	var costTotal float64
	pacer := &progressPacer{}

	for _, file := range files {
		select {
		case <-ctx.Done():
			result.Error = strPtr("下载被取消")
			result.SuccessFiles, result.FailedFiles = success, failed
			result.TotalBytes, result.TotalFiles, result.ExecCostTime = downloadedBytes, totalFiles-skipped, costTotal
			result.Output = buildTransferOutput(totalFiles-skipped, success, failed, lines)
			return result
		default:
		}

		fileStart := time.Now()

		var remoteFilePath, localFilePath string
		if fi.IsDir() {
			remoteFilePath = remotePath + "/" + file.relativePath
			localFilePath = filepath.Join(ipDir, file.relativePath)
		} else {
			remoteFilePath = remotePath
			localFilePath = filepath.Join(ipDir, filepath.Base(remotePath))
		}

		// 符号链接：跳过，不计失败，仅记明细；同时从总量里扣除以保证进度到 100%
		if lfi, lerr := sftpClient.Lstat(remoteFilePath); lerr == nil && lfi.Mode()&os.ModeSymlink != 0 {
			skipped++
			totalBytes -= file.size
			lines = append(lines, fmt.Sprintf("%s: 已跳过（符号链接）", file.relativePath))
			continue
		}

		if err := os.MkdirAll(filepath.Dir(localFilePath), 0o755); err != nil {
			failed++
			msg := fmt.Sprintf("%s: 下载失败 - 创建本地目录失败: %v", file.relativePath, err)
			lines = append(lines, msg)
			result.Error = strPtr(msg)
			if onProgress != nil {
				onProgress(Progress{Seq: seq, IP: c.cfg.IP, DownloadedBytes: downloadedBytes, TotalBytes: totalBytes, TotalFiles: totalFiles, SuccessFiles: success, FailedFiles: failed})
			}
			break
		}

		written, downloadErr := c.sftpDownloadFile(sftpClient, remoteFilePath, localFilePath, seq, totalBytes, totalFiles, onProgress)
		cost := time.Since(fileStart).Seconds()
		costTotal += cost

		if downloadErr != nil {
			failed++
			msg := fmt.Sprintf("%s: 下载失败 - %v", file.relativePath, downloadErr)
			lines = append(lines, msg)
			result.Error = strPtr(msg)
			if onProgress != nil {
				onProgress(Progress{Seq: seq, IP: c.cfg.IP, DownloadedBytes: downloadedBytes, TotalBytes: totalBytes, TotalFiles: totalFiles, SuccessFiles: success, FailedFiles: failed})
			}
			break
		}
		success++
		downloadedBytes += written
		lines = append(lines, fmt.Sprintf("%s: 下载成功 (%.3fs)", file.relativePath, cost))

		if onProgress != nil && pacer.allow() {
			onProgress(Progress{Seq: seq, IP: c.cfg.IP, DownloadedBytes: downloadedBytes, TotalBytes: totalBytes, TotalFiles: totalFiles, SuccessFiles: success, FailedFiles: failed})
		}
	}

	result.Output = buildTransferOutput(totalFiles-skipped, success, failed, lines)
	// 全传输成功 = 至少成功 1 个文件且 0 失败；全部为符号链接则报「没有下载到任何文件」
	if success > 0 && failed == 0 {
		result.ExitCode = intPtr(0)
	} else if success == 0 && failed == 0 && skipped > 0 {
		result.Error = strPtr("没有下载到任何文件（全部为符号链接）")
	}
	result.ExecCostTime = costTotal
	result.TotalBytes = downloadedBytes
	result.TotalFiles = totalFiles - skipped
	result.SuccessFiles = success
	result.FailedFiles = failed
	return result
}

// sftpUploadFile 直接通过 SFTP 写入文件（非 sudo 路径），返回实际写入字节数。
func (c *Client) sftpUploadFile(sftpClient *sftp.Client, localPath, remoteFilePath string, perm os.FileMode, seq int, totalBytes int64, totalFiles int, onProgress func(Progress)) (int64, error) {
	src, err := os.Open(localPath)
	if err != nil {
		return 0, fmt.Errorf("打开本地文件失败: %w", err)
	}
	defer func() { _ = src.Close() }()

	dst, err := sftpClient.Create(remoteFilePath)
	if err != nil {
		return 0, fmt.Errorf("创建远程文件失败: %w", err)
	}
	defer func() { _ = dst.Close() }()

	written, err := copyUpload(dst, src, &progressWriter{
		dst:        dst,
		seq:        seq,
		ip:         c.cfg.IP,
		totalBytes: totalBytes,
		totalFiles: totalFiles,
		callback:   onProgress,
	}, onProgress == nil)
	if err != nil {
		_ = sftpClient.Remove(remoteFilePath) // 删除远程半成品
		return 0, fmt.Errorf("写入远程文件失败: %w", err)
	}
	if err := sftpClient.Chmod(remoteFilePath, perm); err != nil {
		// 权限设置失败不阻断（旧行为）
	}
	return written, nil
}

// sftpUploadWithSudo 经临时目录 + sudo mv 上传（sudo 路径），返回实际写入字节数。
func (c *Client) sftpUploadWithSudo(sftpClient *sftp.Client, localPath, fileName, remotePath string, perm os.FileMode, seq int, totalBytes int64, totalFiles int, onProgress func(Progress)) (int64, error) {
	tmpDir := tmpRoot + randomHex()
	if err := c.runCommand(fmt.Sprintf("sudo mkdir -p '%s' && sudo chmod 777 '%s'", tmpDir, tmpDir)); err != nil {
		return 0, fmt.Errorf("创建临时目录失败: %w", err)
	}
	cleanup := func() { _ = c.runCommand(fmt.Sprintf("sudo rm -rf '%s'", tmpDir)) }

	src, err := os.Open(localPath)
	if err != nil {
		cleanup()
		return 0, fmt.Errorf("打开本地文件失败: %w", err)
	}
	defer func() { _ = src.Close() }()

	tmpFilePath := tmpDir + "/" + fileName
	dst, err := sftpClient.Create(tmpFilePath)
	if err != nil {
		cleanup()
		return 0, fmt.Errorf("创建临时文件失败: %w", err)
	}
	defer func() { _ = dst.Close() }()

	written, err := copyUpload(dst, src, &progressWriter{
		dst:        dst,
		seq:        seq,
		ip:         c.cfg.IP,
		totalBytes: totalBytes,
		totalFiles: totalFiles,
		callback:   onProgress,
	}, onProgress == nil)
	if err != nil {
		cleanup()
		return 0, fmt.Errorf("写入临时文件失败: %w", err)
	}
	if err := sftpClient.Chmod(tmpFilePath, perm); err != nil {
		// 权限设置失败不阻断（旧行为）
	}

	// sudo mv 到目标（引号转义防路径含特殊字符）
	escaped := strings.ReplaceAll(remotePath, "'", `'\''`)
	if err := c.runCommand(fmt.Sprintf("sudo mv '%s' '%s/'", tmpFilePath, escaped)); err != nil {
		cleanup()
		return 0, fmt.Errorf("sudo mv 失败: %w", err)
	}
	cleanup()
	return written, nil
}

// sftpDownloadFile 通过 SFTP 下载单个文件，返回实际下载字节数；失败删除本地半成品。
func (c *Client) sftpDownloadFile(sftpClient *sftp.Client, remoteFilePath, localFilePath string, seq int, totalBytes int64, totalFiles int, onProgress func(Progress)) (int64, error) {
	src, err := sftpClient.Open(remoteFilePath)
	if err != nil {
		return 0, fmt.Errorf("打开远程文件失败: %w", err)
	}
	defer func() { _ = src.Close() }()

	dst, err := os.Create(localFilePath)
	if err != nil {
		return 0, fmt.Errorf("创建本地文件失败: %w", err)
	}
	defer func() { _ = dst.Close() }()

	written, err := copyDownload(dst, src, &progressReader{
		src:        src,
		seq:        seq,
		ip:         c.cfg.IP,
		totalBytes: totalBytes,
		totalFiles: totalFiles,
		callback:   onProgress,
	}, onProgress == nil)
	if err != nil {
		_ = os.Remove(localFilePath)
		return 0, fmt.Errorf("写入本地文件失败: %w", err)
	}
	return written, nil
}

// copyUpload 1MB 缓冲流式上传；plain 为真时不走进度包裹（无回调场景）。
func copyUpload(dst io.Writer, src io.Reader, pw *progressWriter, plain bool) (int64, error) {
	buf := make([]byte, 1024*1024)
	if plain {
		return io.CopyBuffer(dst, src, buf)
	}
	return io.CopyBuffer(pw, src, buf)
}

// copyDownload 1MB 缓冲流式下载；plain 为真时不走进度包裹（无回调场景）。
func copyDownload(dst io.Writer, src io.Reader, pr *progressReader, plain bool) (int64, error) {
	buf := make([]byte, 1024*1024)
	if plain {
		return io.CopyBuffer(dst, src, buf)
	}
	return io.CopyBuffer(dst, pr, buf)
}

// buildTransferOutput 传输明细文本（头部统计 + 逐文件行），不再 base64 编码。
func buildTransferOutput(total, success, failed int, lines []string) string {
	header := fmt.Sprintf("total_files=%d, success_files=%d, failed_files=%d", total, success, failed)
	if len(lines) == 0 {
		return header
	}
	return header + "\n" + strings.Join(lines, "\n")
}

// randomHex 8 位随机 hex（临时目录名）。
func randomHex() string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%x", b)
}
