package ssh

import (
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/pkg/sftp"
)

// tmpRoot sudo 模式下的远端临时目录根（旧实现同名，上传前会整体清理）。
const tmpRoot = "/tmp/.SSHFleet_tmp/"

// UploadFiles 上传本地文件清单到远程目录（对位旧 UploadFiles）。
// 前置：远程目标目录**必须已存在**（用户 2026-09-11 裁定 Q7：工具不建目录，错路径当场报错）。
//
// skipped 是采集阶段被过滤的链接（相对上传根的路径）：上传侧的过滤发生在本地采集
// （`-u` 输入后即可知，无需连服务器），这里只把它写进 Output 明细留痕（spec D51）。
func (c *Client) UploadFiles(ctx context.Context, files []LocalFile, skipped []string, remotePath string, useSudo bool, seq int, onProgress func(Progress)) *Result {
	result := c.newResult(seq)
	defer c.applyBanner(result) // 服务端提示并入报错原文（ADR-0005）
	result.TotalFiles = len(files)

	if !c.connectFor(ctx, result) {
		result.FailedFiles = len(files)
		return result
	}
	defer func() { _ = c.Close() }()

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
			result.Error = strPtr(fmt.Sprintf("本地文件不存在或不可读: %s - %v", f.Rel, err))
			return result
		}
	}

	// sudo 生效判断：root 用户无需 sudo（旧行为）
	effectiveSudo := useSudo && c.cfg.User != "root"

	success, failed := 0, 0
	var uploadedBytes int64
	// 采集阶段被过滤的软链接先写进明细（随 output 字段落盘/进归档；上传侧不在终端打结果明细），
	// 不计成功也不计失败（spec D51）
	lines := make([]string, 0, len(skipped)+len(files))
	for _, s := range skipped {
		lines = append(lines, fmt.Sprintf("%s: 已跳过（符号链接）", s))
	}
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
		// 远端路径 = 目标目录 + 相对上传根的路径（上传目录保留层级，spec D49）
		remoteFilePath := path.Join(remotePath, f.Rel)

		// 远程文件已存在：该文件失败并终止本节点传输（不覆盖，旧行为）
		if _, err := sftpClient.Stat(remoteFilePath); err == nil {
			failed++
			msg := fmt.Sprintf("%s: 上传失败 - 文件已存在", f.Rel)
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
			msg := fmt.Sprintf("%s: 上传失败 - %v", f.Rel, err)
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
			written, uploadErr = c.sftpUploadWithSudo(sftpClient, f.Path, f.Rel, remotePath, localMode, seq, totalBytes, len(files), onProgress)
		}

		cost := time.Since(fileStart).Seconds()
		costTotal += cost

		if uploadErr != nil {
			failed++
			msg := fmt.Sprintf("%s: 上传失败 - %v", f.Rel, uploadErr)
			lines = append(lines, msg)
			result.Error = strPtr(msg)
			if onProgress != nil {
				onProgress(Progress{Seq: seq, IP: c.cfg.IP, SuccessFiles: success, FailedFiles: failed})
			}
			break
		}
		success++
		uploadedBytes += written
		lines = append(lines, fmt.Sprintf("%s: 上传成功 (%.3fs)", f.Rel, cost))

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
	result := c.newResult(seq)
	defer c.applyBanner(result) // 服务端提示并入报错原文（ADR-0005）

	if !c.connectFor(ctx, result) {
		return result
	}
	defer func() { _ = c.Close() }()

	sftpClient, err := sftp.NewClient(c.conn)
	if err != nil {
		result.Error = strPtr("SFTP 客户端创建失败 - " + err.Error())
		return result
	}
	defer func() { _ = sftpClient.Close() }()

	effectiveSudo := useSudo && c.cfg.User != "root"

	// 远程路径预检（sudo 时用 sudo test）；单引号转义与 find/mv 同口径（2026-09-14 审计修复）
	checkCmd := fmt.Sprintf("test -e '%s'", escapeShellArg(remotePath))
	if effectiveSudo {
		checkCmd = fmt.Sprintf("sudo test -e '%s'", escapeShellArg(remotePath))
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
		symlinks   []string // 被过滤的软链接（相对路径），只用于在结果里提示
		totalBytes int64
	)

	// 远端路径本身即软链接：过滤（不下载、不当失败）
	rootIsLink := false
	if lfi, lerr := sftpClient.Lstat(remotePath); lerr == nil && lfi.Mode()&os.ModeSymlink != 0 {
		rootIsLink = true
		symlinks = append(symlinks, filepath.Base(remotePath))
	}

	switch {
	case rootIsLink:
		// 已过滤，落到下面的「无可下载文件」判定

	case fi.IsDir():
		// 一次 find 同时枚举真文件（F）与软链接（L）：软链接**过滤**但要在结果里提示
		//（用户 2026-09-14 裁定）。此前用 `-type f` 单查，软链接被静默丢弃、用户无从得知。
		findCmd := fmt.Sprintf("find '%s' \\( -type f -printf 'F %%s %%P\\n' \\) -o \\( -type l -printf 'L %%P\\n' \\)",
			escapeShellArg(remotePath))
		if effectiveSudo {
			findCmd = "sudo " + findCmd
		}
		output, err := c.runCommandCapture(findCmd)
		if err != nil {
			result.ExitCode = extractExitCode(err)
			result.Error = strPtr(fmt.Sprintf("获取远程文件列表失败: %v", err))
			return result
		}
		for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
			line = strings.TrimSpace(line)
			switch {
			case line == "":
			case strings.HasPrefix(line, "F "):
				parts := strings.SplitN(line[2:], " ", 2)
				if len(parts) != 2 {
					continue
				}
				var size int64
				_, _ = fmt.Sscanf(parts[0], "%d", &size)
				files = append(files, remoteFile{relativePath: parts[1], size: size})
				totalBytes += size
			case strings.HasPrefix(line, "L "):
				symlinks = append(symlinks, line[2:])
			}
		}

	default:
		files = append(files, remoteFile{relativePath: filepath.Base(remotePath), size: fi.Size()})
		totalBytes = fi.Size()
	}

	sort.Strings(symlinks)

	totalFiles := len(files)
	if totalFiles == 0 {
		// 过滤后没有任何可传文件才报错（全文链接与目录本就为空，措辞分开）
		if len(symlinks) > 0 {
			result.Error = strPtr(fmt.Sprintf("没有可下载的文件：目标全部为软链接（已过滤 %d 个）", len(symlinks)))
		} else {
			result.Error = strPtr("远程路径中没有可下载的文件")
		}
		return result
	}

	ipDir := filepath.Join(localPath, c.cfg.IP)
	if err := os.MkdirAll(ipDir, 0o755); err != nil {
		result.Error = strPtr(fmt.Sprintf("创建本地目录失败: %v", err))
		return result
	}

	success, failed := 0, 0
	var downloadedBytes int64
	// 被过滤的软链接先写进明细（随 output 字段落盘 / 进归档），不计成功也不计失败
	lines := make([]string, 0, len(symlinks)+len(files))
	for _, s := range symlinks {
		lines = append(lines, fmt.Sprintf("%s: 已跳过（符号链接）", s))
	}
	var costTotal float64
	pacer := &progressPacer{}

	for _, file := range files {
		select {
		case <-ctx.Done():
			result.Error = strPtr("下载被取消")
			result.SuccessFiles, result.FailedFiles = success, failed
			result.TotalBytes, result.TotalFiles, result.ExecCostTime = downloadedBytes, totalFiles, costTotal
			result.Output = buildTransferOutput(totalFiles, success, failed, lines)
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

	result.Output = buildTransferOutput(totalFiles, success, failed, lines)
	// 全传输成功 = 至少成功 1 个文件且 0 失败；「一个都没传」的情况已在枚举阶段拦下
	if success > 0 && failed == 0 {
		result.ExitCode = intPtr(0)
	}
	result.ExecCostTime = costTotal
	result.TotalBytes = downloadedBytes
	result.TotalFiles = totalFiles
	result.SuccessFiles = success
	result.FailedFiles = failed
	return result
}

// sftpUploadFile 直接通过 SFTP 写入文件（非 sudo 路径），返回实际写入字节数。
func (c *Client) sftpUploadFile(sftpClient *sftp.Client, localPath, remoteFilePath string, perm os.FileMode, seq int, totalBytes int64, totalFiles int, onProgress func(Progress)) (int64, error) {
	// 中间目录按需创建：上传目录的相对层级要保留（spec D49）。
	// 目标目录（-p）本身仍必须已存在——这里只建它下面的层级，不建它在远端的位置。
	if dir := path.Dir(remoteFilePath); dir != "." {
		if err := sftpClient.MkdirAll(dir); err != nil {
			return 0, fmt.Errorf("创建远程目录失败 %s: %w", dir, err)
		}
	}

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
func (c *Client) sftpUploadWithSudo(sftpClient *sftp.Client, localPath, rel, remotePath string, perm os.FileMode, seq int, totalBytes int64, totalFiles int, onProgress func(Progress)) (int64, error) {
	// 最终落点 = 目标目录 + 相对路径；中间的层级用 sudo 建（目标目录本身必须已存在）
	remoteFilePath := path.Join(remotePath, rel)
	if dir := path.Dir(remoteFilePath); dir != remotePath {
		if err := c.runCommand(fmt.Sprintf("sudo mkdir -p '%s'", escapeShellArg(dir))); err != nil {
			return 0, fmt.Errorf("创建远程目录失败 %s: %w", dir, err)
		}
	}

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

	// 临时文件名用基名，避免相对路径里的分隔符在临时目录里再建一层
	tmpFilePath := tmpDir + "/" + path.Base(rel)
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

	// sudo mv 到最终路径（引号转义防路径含特殊字符）
	if err := c.runCommand(fmt.Sprintf("sudo mv '%s' '%s'", tmpFilePath, escapeShellArg(remoteFilePath))); err != nil {
		cleanup()
		return 0, fmt.Errorf("sudo mv 失败: %w", err)
	}
	cleanup()
	return written, nil
}

// escapeShellArg 把路径嵌进远端命令的单引号里之前的转义。
func escapeShellArg(s string) string {
	return strings.ReplaceAll(s, "'", `'\''`)
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
