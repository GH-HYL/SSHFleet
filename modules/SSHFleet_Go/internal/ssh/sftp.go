package ssh

import (
	"context"
	"crypto/rand"
	"errors"
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
	defer c.captureBanner(result) // 服务端提示无条件写独立字段（D34）
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
		result.Error = strPtr(Message(EnvTransfer, transferClientFailed, err.Error()))
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
		// 措辞与下载侧（下方"远程路径不存在"）及失败分类同名——分类按报错原文
		// 匹配关键字，两处各写一套就会有一条永远归不了类。
		result.Error = strPtr(Message(EnvTransfer, transferRemoteMissing, "remote path not found: "+remotePath))
		return result
	}
	if !fi.IsDir() {
		result.Error = strPtr(Message(EnvTransfer, transferRemoteNotDir, "remote target is not a directory: "+remotePath))
		return result
	}

	// 本地文件预检
	for _, f := range files {
		if _, err := os.Stat(f.Path); err != nil {
			result.FailedFiles = len(files)
			result.Error = strPtr(Message(EnvTransfer, transferLocalUnreadable, fmt.Sprintf("%s: %v", f.Rel, err)))
			return result
		}
	}

	// sudo 生效判断：root 用户无需 sudo（旧行为）
	effectiveSudo := useSudo && c.cfg.User != "root"

	success, failed := 0, 0
	var uploadedBytes int64
	// writtenPaths 本次已写成功的远端完整路径。失败回滚只删这些——不扫目录、不用通配符
	// （ADR-0022 决定第 2 条）：写入前必查重名，所以这里的每一条都是本次自己建的。
	writtenPaths := make([]string, 0, len(files))
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
			// 被中断同样算「这次没传完」（ADR-0022 第 1 条：要么全传上去、要么一个都不留）：
			// 已经写成功的那些照样回滚，不能因为「是用户按的中断」就把残留留在目标机上。
			result.Canceled = true
			result.SuccessFiles, result.FailedFiles = success, failed
			result.TotalBytes, result.ExecCostTime = uploadedBytes, costTotal
			result.Output = buildTransferOutput(len(files), success, failed, lines)
			result.Error = appendMessage(result.Error, c.rollbackUpload(sftpClient, writtenPaths, useSudo))
			result.Error = appendAbort(result.Error, true)
			return result
		default:
		}

		fileStart := time.Now()
		// 远端路径 = 目标目录 + 相对上传根的路径（上传目录保留层级，spec D49）
		remoteFilePath := path.Join(remotePath, f.Rel)

		// 远程文件已存在：该文件失败并终止本节点传输（不覆盖，旧行为）。
		// 注意：这个文件**没有**被写、也不进 writtenPaths——目标机上那个同名文件是别人先放的，
		// 回滚一定不能碰它。
		if _, err := sftpClient.Stat(remoteFilePath); err == nil {
			failed++
			// 报错原文格只放报文（ADR-0021 第 8 条：执行侧只陈述事实，不下结论）。
			// 「上传失败」这类中文结论只留在输出明细行（lines），不参与判据匹配。
			msg := fmt.Sprintf("%s: 上传失败 - %s", f.Rel, Message(EnvTransfer, transferRemoteExists, "file already exists on the remote host"))
			lines = append(lines, msg)
			result.Error = strPtr(fmt.Sprintf("%s: %s", f.Rel, Message(EnvTransfer, transferRemoteExists, "file already exists on the remote host")))
			if onProgress != nil {
				onProgress(Progress{Seq: seq, IP: c.cfg.IP, SuccessFiles: success, FailedFiles: failed})
			}
			result.Error = appendMessage(result.Error, c.rollbackUpload(sftpClient, writtenPaths, useSudo))
			break
		}

		localInfo, err := os.Stat(f.Path)
		if err != nil {
			failed++
			msg := fmt.Sprintf("%s: 上传失败 - %v", f.Rel, err)
			lines = append(lines, msg)
			result.Error = strPtr(Message(EnvTransfer, transferLocalUnreadable, fmt.Sprintf("%s: %v", f.Rel, err)))
			if onProgress != nil {
				onProgress(Progress{Seq: seq, IP: c.cfg.IP, SuccessFiles: success, FailedFiles: failed})
			}
			result.Error = appendMessage(result.Error, c.rollbackUpload(sftpClient, writtenPaths, useSudo))
			break
		}
		localMode := localInfo.Mode().Perm()

		var (
			written    int64
			uploadErr  error
			halfWrites []string
		)
		if !effectiveSudo {
			written, uploadErr, halfWrites = c.sftpUploadFile(sftpClient, f.Path, remoteFilePath, localMode, seq, totalBytes, len(files), onProgress)
		} else {
			written, uploadErr, halfWrites = c.sftpUploadWithSudo(sftpClient, f.Path, f.Rel, remotePath, localMode, seq, totalBytes, len(files), onProgress)
		}

		cost := time.Since(fileStart).Seconds()
		costTotal += cost

		if uploadErr != nil {
			failed++
			msg := fmt.Sprintf("%s: 上传失败 - %v", f.Rel, uploadErr)
			lines = append(lines, msg)
			// uploadErr 由上传函数产出，已经是报文；万一不是（将来新增的路径漏了包），
			// 在这里兜一层，别让中文结论或者裸原文进报错原文格。
			reason := uploadErr.Error()
			if !IsMessage(reason) {
				reason = Message(EnvTransfer, transferRemoteFileFailed, reason)
			}
			result.Error = strPtr(fmt.Sprintf("%s: %s", f.Rel, reason))
			if onProgress != nil {
				onProgress(Progress{Seq: seq, IP: c.cfg.IP, SuccessFiles: success, FailedFiles: failed})
			}
			// 失败即回滚（ADR-0022）：目标机恢复成本次执行前的样子。
			// 失败文件自己的半成品由上传函数删掉，这里只清前面写成功的那些。
			result.Error = appendMessage(result.Error, c.rollbackUpload(sftpClient, writtenPaths, useSudo))
			// 半成品没删掉时照实说（工单 09）：它们不在回滚清单里，混进回滚计数会失真。
			result.Error = appendMessage(result.Error, PartialLeftMessage(halfWrites))
			break
		}
		success++
		// 只有**写成功**的才进清单——回滚删的就是这份清单，多记一条就多一分误删风险。
		writtenPaths = append(writtenPaths, remoteFilePath)
		uploadedBytes += written
		// 逐个文件的「成功」行不写：明细首行的文件总数 / 成功 / 失败已说全，每行再带一次
		// 耗时是重复输出（2026-10-07 作者定）。失败行与跳过行照旧——它们各自带独立信息。

		if onProgress != nil && pacer.allow() {
			onProgress(Progress{Seq: seq, IP: c.cfg.IP, UploadedBytes: uploadedBytes, TotalBytes: totalBytes, TotalFiles: len(files), SuccessFiles: success, FailedFiles: failed})
		}
	}

	result.Output = buildTransferOutput(len(files), success, failed, lines)
	// 传输不是命令执行：不写任何退出码，成败由结果判定按 FailedFiles 等事实给
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
	defer c.captureBanner(result) // 服务端提示无条件写独立字段（D34）

	if !c.connectFor(ctx, result) {
		return result
	}
	defer func() { _ = c.Close() }()

	sftpClient, err := sftp.NewClient(c.conn)
	if err != nil {
		result.Error = strPtr(Message(EnvTransfer, transferClientFailed, err.Error()))
		return result
	}
	defer func() { _ = sftpClient.Close() }()

	effectiveSudo := useSudo && c.cfg.User != "root"

	// 远程路径预检（sudo 时用 sudo test）；路径经 escapeShellArg 转义，转义规则与 find/mv 同源
	checkCmd := fmt.Sprintf("test -e '%s'", escapeShellArg(remotePath))
	if effectiveSudo {
		checkCmd = fmt.Sprintf("sudo test -e '%s'", escapeShellArg(remotePath))
	}
	if err := c.runCommand(checkCmd); err != nil {
		// 预检查是中间步骤（D30）：它自己的退出码进 Steps，不顶成结果退出码——
		// 判定据此走第一组、按报错原文归类（D27 消掉第二组同名补丁的根据就在这）
		result.Steps = append(result.Steps, StepResult{Name: StepPrecheck, ExitCode: extractExitCode(err)})
		result.Error = strPtr(Message(EnvTransfer, transferRemoteMissing, "remote path not found: "+remotePath))
		return result
	}

	fi, err := sftpClient.Stat(remotePath)
	if err != nil {
		result.Error = strPtr(Message(EnvTransfer, transferRemoteUnreachable, "remote path not accessible: "+remotePath))
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
			// 枚举也是中间步骤：退出码进 Steps，理由同上
			result.Steps = append(result.Steps, StepResult{Name: StepEnumerate, ExitCode: extractExitCode(err)})
			result.Error = strPtr(Message(EnvTransfer, transferListFailed, err.Error()))
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
			result.Error = strPtr(Message(EnvTransfer, transferNoFiles, fmt.Sprintf("all %d entries are symlinks (filtered)", len(symlinks))))
		} else {
			result.Error = strPtr(Message(EnvTransfer, transferNoFiles, "no files to download under "+remotePath))
		}
		return result
	}

	// 本地落地目录：开工后才建，失败照样发报文。参数阶段只确认过 -p 存在，
	// 从确认屏到真正开传之间它可能已被删、被改权限——这类失败必须落到「本地文件错误」，
	// 不能因为「前面查过了」就不报。
	ipDir := filepath.Join(localPath, c.cfg.IP)
	if err := os.MkdirAll(ipDir, 0o755); err != nil {
		result.Error = strPtr(Message(EnvTransfer, transferLocalDirFailed, err.Error()))
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
			// 同上传侧：只记结构事实，并陈述「在哪一步被中断」（见上传循环处说明）。
			result.Canceled = true
			result.SuccessFiles, result.FailedFiles = success, failed
			result.TotalBytes, result.TotalFiles, result.ExecCostTime = downloadedBytes, totalFiles, costTotal
			result.Output = buildTransferOutput(totalFiles, success, failed, lines)
			result.Error = appendAbort(result.Error, true)
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
			msg := fmt.Sprintf("%s: 下载失败 - %s", file.relativePath, Message(EnvTransfer, transferLocalDirFailed, err.Error()))
			lines = append(lines, msg)
			result.Error = strPtr(msg)
			if onProgress != nil {
				onProgress(Progress{Seq: seq, IP: c.cfg.IP, DownloadedBytes: downloadedBytes, TotalBytes: totalBytes, TotalFiles: totalFiles, SuccessFiles: success, FailedFiles: failed})
			}
			break
		}

		written, downloadErr, halfWrites := c.sftpDownloadFile(sftpClient, remoteFilePath, localFilePath, seq, totalBytes, totalFiles, onProgress)
		cost := time.Since(fileStart).Seconds()
		costTotal += cost

		if downloadErr != nil {
			failed++
			// 「下载失败」只留在输出明细行；报错原文格放报文（ADR-0021 第 8 条）。
			msg := fmt.Sprintf("%s: 下载失败 - %v", file.relativePath, downloadErr)
			lines = append(lines, msg)
			reason := downloadErr.Error()
			if !IsMessage(reason) {
				reason = Message(EnvTransfer, transferLocalFileFailed, reason)
			}
			result.Error = strPtr(fmt.Sprintf("%s: %s", file.relativePath, reason))
			if onProgress != nil {
				onProgress(Progress{Seq: seq, IP: c.cfg.IP, DownloadedBytes: downloadedBytes, TotalBytes: totalBytes, TotalFiles: totalFiles, SuccessFiles: success, FailedFiles: failed})
			}
			result.Error = appendMessage(result.Error, LocalPartialLeftMessage(halfWrites))
			break
		}
		success++
		downloadedBytes += written
		// 同上传侧：逐文件的「成功」行不写，明细首行已给出总数 / 成功 / 失败（2026-10-07 作者定）。

		if onProgress != nil && pacer.allow() {
			onProgress(Progress{Seq: seq, IP: c.cfg.IP, DownloadedBytes: downloadedBytes, TotalBytes: totalBytes, TotalFiles: totalFiles, SuccessFiles: success, FailedFiles: failed})
		}
	}

	result.Output = buildTransferOutput(totalFiles, success, failed, lines)
	// 传输不是命令执行：不写任何退出码；「至少成功 1 个且 0 失败才算成功」由结果判定按
	// SuccessFiles / FailedFiles 给（「一个都没传」的情况已在枚举阶段拦下）
	result.ExecCostTime = costTotal
	result.TotalBytes = downloadedBytes
	result.TotalFiles = totalFiles
	result.SuccessFiles = success
	result.FailedFiles = failed
	return result
}

// sftpUploadFile 直接通过 SFTP 写入文件（非 sudo 路径），返回实际写入字节数。
func (c *Client) sftpUploadFile(sftpClient *sftp.Client, localPath, remoteFilePath string, perm os.FileMode, seq int, totalBytes int64, totalFiles int, onProgress func(Progress)) (int64, error, []string) {
	// 中间目录按需创建：上传目录的相对层级要保留（spec D49）。
	// 目标目录（-p）本身仍必须已存在——这里只建它下面的层级，不建它在远端的位置。
	if dir := path.Dir(remoteFilePath); dir != "." {
		if err := sftpClient.MkdirAll(dir); err != nil {
			return 0, errors.New(Message(EnvTransfer, transferRemoteFileFailed, fmt.Sprintf("create remote dir %s: %v", dir, err))), nil
		}
	}

	src, err := os.Open(localPath)
	if err != nil {
		return 0, errors.New(Message(EnvTransfer, transferLocalUnreadable, err.Error())), nil
	}
	defer func() { _ = src.Close() }()

	dst, err := sftpClient.Create(remoteFilePath)
	if err != nil {
		return 0, errors.New(Message(EnvTransfer, transferRemoteFileFailed, err.Error())), nil
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
		var leftovers []string
		if rmErr := sftpClient.Remove(remoteFilePath); rmErr != nil { // 删除远程半成品
			// 删不掉就照实说：它在目标机上占着路径，下次重传会撞「远程文件已存在」。
			leftovers = append(leftovers, remoteFilePath)
		}
		return 0, errors.New(Message(EnvTransfer, transferRemoteFileFailed, err.Error())), leftovers
	}
	if err := sftpClient.Chmod(remoteFilePath, perm); err != nil {
		// 权限设置失败不阻断（旧行为）
	}
	return written, nil, nil
}

// sftpUploadWithSudo 经临时目录 + sudo mv 上传（sudo 路径），返回实际写入字节数。
func (c *Client) sftpUploadWithSudo(sftpClient *sftp.Client, localPath, rel, remotePath string, perm os.FileMode, seq int, totalBytes int64, totalFiles int, onProgress func(Progress)) (int64, error, []string) {
	// 最终落点 = 目标目录 + 相对路径；中间的层级用 sudo 建（目标目录本身必须已存在）
	remoteFilePath := path.Join(remotePath, rel)
	if dir := path.Dir(remoteFilePath); dir != remotePath {
		if err := c.runCommand(fmt.Sprintf("sudo mkdir -p '%s'", escapeShellArg(dir))); err != nil {
			return 0, errors.New(Message(EnvTransfer, transferRemoteFileFailed, fmt.Sprintf("create remote dir %s: %v", dir, err))), nil
		}
	}

	tmpDir := tmpRoot + randomHex()
	if err := c.runCommand(fmt.Sprintf("sudo mkdir -p '%s' && sudo chmod 777 '%s'", tmpDir, tmpDir)); err != nil {
		return 0, errors.New(Message(EnvTransfer, transferTmpFailed, err.Error())), nil
	}
	// cleanup 的失败不再静默：临时目录清不掉时，目标机上会留着本次的半成品。
	var leftovers []string
	cleanup := func() {
		if err := c.runCommand(fmt.Sprintf("sudo rm -rf '%s'", tmpDir)); err != nil {
			if len(leftovers) == 0 || leftovers[len(leftovers)-1] != tmpDir {
				leftovers = append(leftovers, tmpDir)
			}
		}
	}

	src, err := os.Open(localPath)
	if err != nil {
		cleanup()
		return 0, errors.New(Message(EnvTransfer, transferLocalUnreadable, err.Error())), leftovers
	}
	defer func() { _ = src.Close() }()

	// 临时文件名用基名，避免相对路径里的分隔符在临时目录里再建一层
	tmpFilePath := tmpDir + "/" + path.Base(rel)
	dst, err := sftpClient.Create(tmpFilePath)
	if err != nil {
		cleanup()
		return 0, errors.New(Message(EnvTransfer, transferTmpFailed, err.Error())), leftovers
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
		return 0, errors.New(Message(EnvTransfer, transferTmpFailed, err.Error())), leftovers
	}
	if err := sftpClient.Chmod(tmpFilePath, perm); err != nil {
		// 权限设置失败不阻断（旧行为）
	}

	// sudo mv 到最终路径：两个参数都转义——两边都含用户可控字符（临时名里带着本地文件名的
	// 基名，最终名是原路径 + 原文件名），任一边漏转义都会把远端命令拼坏。
	if err := c.runCommand(fmt.Sprintf("sudo mv '%s' '%s'", escapeShellArg(tmpFilePath), escapeShellArg(remoteFilePath))); err != nil {
		// 跨文件系统时 mv 退化为 copy+unlink，中途失败可能在最终位置留下半成品。
		// 不猜：先 Stat，确认存在才报，别把「根本没写出去」误报成残留。
		if _, statErr := sftpClient.Stat(remoteFilePath); statErr == nil {
			leftovers = append(leftovers, remoteFilePath)
		}
		cleanup()
		return 0, errors.New(Message(EnvTransfer, transferSudoFailed, err.Error())), leftovers
	}
	cleanup()
	return written, nil, leftovers
}

// rollbackUpload 上传失败后删掉本次已写成功的文件（ADR-0022）。
//
// 只删 writtenPaths 里列的那些完整路径，不扫目录、不用通配符、不删中间目录：
// 上传流程在写每个文件之前都查过重名，所以这些路径必然都是本次自己建的。
// 删不掉的路径照实带进报文，不静默——下次撞上「远程文件已存在」时，使用者要能
// 回头看出上次没清干净。
//
// sudo 路径用 sudo rm（写进去的文件可能属主是 root）；非 sudo 用 SFTP Remove。
func (c *Client) rollbackUpload(sftpClient *sftp.Client, writtenPaths []string, useSudo bool) string {
	remove := func(p string) error {
		if useSudo {
			return c.runCommand(fmt.Sprintf("sudo rm -f '%s'", escapeShellArg(p)))
		}
		return sftpClient.Remove(p)
	}
	return rollbackWith(writtenPaths, remove)
}

// rollbackWith 逐条点名删除，把结果说成一句话（与「怎么删」解耦，便于单测删不掉的情形）。
//
// 删除的动作由调用方给：非 sudo 是 SFTP Remove、sudo 是 `sudo rm -f`。这里只负责
// 「一条一条删、成功的计数、失败的留路径」，**绝不扩大删除范围**。
func rollbackWith(writtenPaths []string, remove func(p string) error) string {
	if len(writtenPaths) == 0 {
		return ""
	}
	left := make([]string, 0)
	removed := 0
	for _, p := range writtenPaths {
		if err := remove(p); err != nil {
			left = append(left, p)
			continue
		}
		removed++
	}
	return RollbackMessage(len(writtenPaths), removed, left)
}

// escapeShellArg 把路径嵌进远端命令的单引号里之前的转义。
// 转义规则与 shellQuote 同源（escapeSingleQuotes）——不再各写一遍，与 find/mv 的
// 口径由结构保证，不靠注释提醒人。
func escapeShellArg(s string) string {
	return escapeSingleQuotes(s)
}

// sftpDownloadFile 通过 SFTP 下载单个文件，返回实际下载字节数；失败删除本地半成品。
func (c *Client) sftpDownloadFile(sftpClient *sftp.Client, remoteFilePath, localFilePath string, seq int, totalBytes int64, totalFiles int, onProgress func(Progress)) (int64, error, []string) {
	// 打开的是**远端**文件：这一步失败是目标机那边的事（文件刚被删、权限不够），
	// 挂 remote_file_failed。挂 local_file_failed 会让「本机写不进去」与「远端读不出来」
	// 共用同一个环节名，使用者按分类排查时会找错方向。
	src, err := sftpClient.Open(remoteFilePath)
	if err != nil {
		return 0, errors.New(Message(EnvTransfer, transferRemoteFileFailed, err.Error())), nil
	}
	defer func() { _ = src.Close() }()

	dst, err := os.Create(localFilePath)
	if err != nil {
		return 0, errors.New(Message(EnvTransfer, transferLocalFileFailed, err.Error())), nil
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
		var leftovers []string
		if rmErr := os.Remove(localFilePath); rmErr != nil {
			// 本机半成品没删掉：下次重试会撞上这个残缺文件（工单 09）。
			leftovers = append(leftovers, localFilePath)
		}
		return 0, errors.New(Message(EnvTransfer, transferLocalFileFailed, err.Error())), leftovers
	}
	return written, nil, nil
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
// 逐文件行只剩"出过事的那几个"——失败与符号链接跳过；逐个成功不再逐行列出（2026-10-07 作者定）。
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
