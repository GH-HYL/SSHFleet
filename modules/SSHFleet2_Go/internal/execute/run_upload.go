package execute

// 上传：把采集好的那张表写到目标目录。
//
// 三条硬规矩（ADR-0022）：
//   - 远端重名不覆盖，那个文件计失败并终止本节点（目标机上那个同名文件是别人先
//     放的，回滚一定不能碰它）；
//   - 中途失败把**本次写成功**的那些删掉（只删清单里的路径，不扫目录、不用通配符）；
//   - sudo 路径先写进远端暂存目录再 `mv` 到位，暂存目录用完清理。
//
// 明细进 `terminal-output.txt`，不上屏——传输类的结果由进度面板呈现（老版同一判据）。

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	"path"
	"time"

	"github.com/pkg/sftp"

	"sshfleet2/internal/contract"
)

// upload 一台的上传。
func (c *client) upload(ctx context.Context, p *contract.Plan, res *contract.NodeResult, onProg onProgressN) {
	// 传输那一段的耗时：老版记的是「从开工到收场」这一段，失败也算——屏幕明细与执行期
	// 日志都读它（`res.Cost`），命令与改密各自在入口处记，这里与下载同一处口径。
	start := time.Now()
	defer func() { res.Cost = time.Since(start) }()

	task := p.Task.Upload
	files := make([]fileEntry, 0, len(task.Files))
	for _, f := range task.Files {
		files = append(files, fileEntry{Path: f.Path, Rel: f.Rel, Size: f.Size})
	}
	res.Transfer.TotalFiles = len(files)

	remotePath := p.Task.Path
	useSudo := p.Run.Sudo && c.node.User != "root"

	// 清理残留临时目录（仅 sudo 模式）
	if useSudo {
		_ = c.exec("sudo rm -rf " + tmpRoot)
	}

	sftpClient, err := sftp.NewClient(c.conn)
	if err != nil {
		res.ErrorText = appendText(res.ErrorText, msg(envTransfer, transferClientFailed, err.Error()))
		res.Transfer.FailFiles = len(files)
		return
	}
	defer func() { _ = sftpClient.Close() }()

	var totalBytes int64
	for _, f := range files {
		totalBytes += f.Size
	}

	// 远程目标路径检查：必须存在且是目录（工具不建 -p 指的目录）。
	fi, err := sftpClient.Stat(remotePath)
	if err != nil {
		// 措辞与下载侧同名——分类按报错原文匹配关键字，两处各写一套就会有一条永远归不了类。
		res.ErrorText = appendText(res.ErrorText, msg(envTransfer, transferRemoteMissing, "remote path not found: "+remotePath))
		res.Transfer.FailFiles = len(files)
		return
	}
	if !fi.IsDir() {
		res.ErrorText = appendText(res.ErrorText, msg(envTransfer, transferRemoteNotDir, "remote target is not a directory: "+remotePath))
		res.Transfer.FailFiles = len(files)
		return
	}

	// 本地文件预检：本地读不了是更近、更确定的原因，不该被「磁盘满」抢先报出来。
	for _, f := range files {
		if _, err := os.Stat(f.Path); err != nil {
			res.Transfer.FailFiles = len(files)
			res.ErrorText = appendText(res.ErrorText, msg(envTransfer, transferLocalUnreadable, fmt.Sprintf("%s: %v", f.Rel, err)))
			return
		}
	}

	// 空间预检：装不下就停在写第一个文件之前，目标机上不会有任何本次的东西。
	if m := spaceShort(sftpProbe(sftpClient), remotePath, useSudo, totalBytes, maxFileSize(files), true); m != "" {
		res.ErrorText = appendText(res.ErrorText, m)
		res.Transfer.FailFiles = len(files)
		return
	}

	success, failed := 0, 0
	var uploadedBytes int64
	// 本次已写成功的远端完整路径。失败回滚只删这些——不扫目录、不用通配符。
	written := make([]string, 0, len(files))
	lines := make([]string, 0, len(task.Skipped)+len(files))
	for _, s := range task.Skipped {
		lines = append(lines, fmt.Sprintf("%s: 已跳过（符号链接）", s))
	}
	var pacer progressPacer

	for idx, f := range files {
		if ctx.Err() != nil {
			// 被中断同样算「这次没传完」：已经写成功的照样回滚，不能因为「是用户按的
			// 中断」就把残留留在目标机上。
			res.Cancelled = true
			res.Transfer.SuccessFiles, res.Transfer.FailFiles = success, failed
			res.Transfer.TotalBytes = uploadedBytes
			res.Output = buildTransferOutput(len(files), success, failed, lines)
			res.ErrorText = appendText(res.ErrorText, c.rollback(sftpClient, written, useSudo))
			res.ErrorText = appendText(res.ErrorText, msg(envAbort, abortCanceled, "interrupted after session established"))
			return
		}

		remoteFilePath := path.Join(remotePath, f.Rel)

		// 远端重名：不覆盖、计失败、终止本节点。
		if _, err := sftpClient.Stat(remoteFilePath); err == nil {
			failed++
			reason := msg(envTransfer, transferRemoteExists, "file already exists on the remote host")
			lines = append(lines, fmt.Sprintf("%s: 上传失败 - %s", f.Rel, reason))
			res.ErrorText = appendText(res.ErrorText, fmt.Sprintf("%s: %s", f.Rel, reason))
			res.ErrorText = appendText(res.ErrorText, c.rollback(sftpClient, written, useSudo))
			break
		}

		localInfo, statErr := os.Stat(f.Path)
		if statErr != nil {
			failed++
			lines = append(lines, fmt.Sprintf("%s: 上传失败 - %v", f.Rel, statErr))
			res.ErrorText = appendText(res.ErrorText, msg(envTransfer, transferLocalUnreadable, fmt.Sprintf("%s: %v", f.Rel, statErr)))
			res.ErrorText = appendText(res.ErrorText, c.rollback(sftpClient, written, useSudo))
			break
		}
		perm := localInfo.Mode().Perm()

		var (
			writtenBytes int64
			uploadErr    error
			halfWrites   []string
		)
		if !useSudo {
			writtenBytes, uploadErr, halfWrites = c.putFile(sftpClient, f.Path, remoteFilePath, perm, idx, totalBytes, len(files), onProg)
		} else {
			writtenBytes, uploadErr, halfWrites = c.putFileViaSudo(sftpClient, f.Path, f.Rel, remotePath, perm, idx, totalBytes, len(files), onProg)
		}

		if uploadErr != nil {
			failed++
			lines = append(lines, fmt.Sprintf("%s: 上传失败 - %v", f.Rel, uploadErr))
			reason := uploadErr.Error()
			if !isMessage(reason) {
				reason = msg(envTransfer, transferRemoteFileFailed, reason)
			}
			res.ErrorText = appendText(res.ErrorText, fmt.Sprintf("%s: %s", f.Rel, reason))
			// 失败后再查一次空间：满不是唯一的原因，只有探出来确实不够才改口。
			// **必须在回滚之前问**：回滚会把空间还回去，那时再探永远是「够」。
			res.ErrorText = appendText(res.ErrorText,
				spaceShort(sftpProbe(sftpClient), remotePath, useSudo, totalBytes-uploadedBytes, maxFileSize(files[idx:]), false))
			res.ErrorText = appendText(res.ErrorText, c.rollback(sftpClient, written, useSudo))
			// 半成品没删掉时照实说：它们不在回滚清单里，混进回滚计数会失真。
			res.ErrorText = appendText(res.ErrorText, partialLeft(halfWrites))
			break
		}
		success++
		// 只有**写成功**的才进清单——回滚删的就是这份清单，多记一条就多一分误删风险。
		written = append(written, remoteFilePath)
		uploadedBytes += writtenBytes

		// 逐文件进展也报上去：文件小、字节级回调太稀时，面板靠它动。
		if pacer.allow() {
			reportTransfer(onProg, idx, c.node.IP, uploadedBytes, totalBytes, len(files), success, failed)
		}
	}

	res.Output = buildTransferOutput(len(files), success, failed, lines)
	res.Transfer.SuccessFiles = success
	res.Transfer.FailFiles = failed
	res.Transfer.TotalBytes = uploadedBytes
}

// putFile 直接经 SFTP 写（非 sudo 路径），返回写入字节数与没删掉的半成品。
func (c *client) putFile(sftpClient *sftp.Client, localPath, remoteFilePath string, perm os.FileMode, seq int, totalBytes int64, totalFiles int, onProg onProgressN) (int64, error, []string) {
	// 中间目录按需创建：上传目录的相对层级要保留。目标目录（-p）本身仍必须已存在。
	if dir := path.Dir(remoteFilePath); dir != "." {
		if err := sftpClient.MkdirAll(dir); err != nil {
			return 0, errors.New(msg(envTransfer, transferRemoteFileFailed, fmt.Sprintf("create remote dir %s: %v", dir, err))), nil
		}
	}

	src, err := os.Open(localPath)
	if err != nil {
		return 0, errors.New(msg(envTransfer, transferLocalUnreadable, err.Error())), nil
	}
	defer func() { _ = src.Close() }()

	dst, err := sftpClient.Create(remoteFilePath)
	if err != nil {
		return 0, errors.New(msg(envTransfer, transferRemoteFileFailed, err.Error())), nil
	}
	defer func() { _ = dst.Close() }()

	n, err := copyUpload(dst, src, onProg, seq, c.node.IP, totalBytes, totalFiles)
	if err != nil {
		var leftovers []string
		if rmErr := sftpClient.Remove(remoteFilePath); rmErr != nil {
			// 删不掉就照实说：它在目标机上占着路径，下次重传会撞「远程文件已存在」。
			leftovers = append(leftovers, remoteFilePath)
		}
		return 0, errors.New(msg(envTransfer, transferRemoteFileFailed, err.Error())), leftovers
	}
	// 权限设置失败不阻断（老版同口径）。
	_ = sftpClient.Chmod(remoteFilePath, perm)
	return n, nil, nil
}

// putFileViaSudo 经暂存目录 + `sudo mv` 写（sudo 路径）。
func (c *client) putFileViaSudo(sftpClient *sftp.Client, localPath, rel, remotePath string, perm os.FileMode, seq int, totalBytes int64, totalFiles int, onProg onProgressN) (int64, error, []string) {
	remoteFilePath := path.Join(remotePath, rel)
	if dir := path.Dir(remoteFilePath); dir != remotePath {
		if err := c.exec(fmt.Sprintf("sudo mkdir -p '%s'", escapeShellArg(dir))); err != nil {
			return 0, errors.New(msg(envTransfer, transferRemoteFileFailed, fmt.Sprintf("create remote dir %s: %v", dir, err))), nil
		}
	}

	tmpDir := tmpRoot + randomHex()
	if err := c.exec(fmt.Sprintf("sudo mkdir -p '%s' && sudo chmod 777 '%s'", tmpDir, tmpDir)); err != nil {
		return 0, errors.New(msg(envTransfer, transferTmpFailed, err.Error())), nil
	}
	// 临时目录清不掉时，目标机上会留着本次的半成品——不静默。
	var leftovers []string
	cleanup := func() {
		if err := c.exec(fmt.Sprintf("sudo rm -rf '%s'", tmpDir)); err != nil {
			if len(leftovers) == 0 || leftovers[len(leftovers)-1] != tmpDir {
				leftovers = append(leftovers, tmpDir)
			}
		}
	}

	src, err := os.Open(localPath)
	if err != nil {
		cleanup()
		return 0, errors.New(msg(envTransfer, transferLocalUnreadable, err.Error())), leftovers
	}
	defer func() { _ = src.Close() }()

	// 临时文件名用基名：避免相对路径里的分隔符在临时目录里再建一层。
	tmpFilePath := tmpDir + "/" + path.Base(rel)
	dst, err := sftpClient.Create(tmpFilePath)
	if err != nil {
		cleanup()
		return 0, errors.New(msg(envTransfer, transferTmpFailed, err.Error())), leftovers
	}
	defer func() { _ = dst.Close() }()

	n, err := copyUpload(dst, src, onProg, seq, c.node.IP, totalBytes, totalFiles)
	if err != nil {
		cleanup()
		return 0, errors.New(msg(envTransfer, transferTmpFailed, err.Error())), leftovers
	}
	_ = sftpClient.Chmod(tmpFilePath, perm)

	// 两个参数都转义：两边都含用户可控字符，任一边漏转义都会把远端命令拼坏。
	if err := c.exec(fmt.Sprintf("sudo mv '%s' '%s'", escapeShellArg(tmpFilePath), escapeShellArg(remoteFilePath))); err != nil {
		// 跨文件系统时 mv 退化为 copy+unlink，中途失败可能在最终位置留下半成品。
		// 不猜：先 Stat，确认存在才报。
		if _, statErr := sftpClient.Stat(remoteFilePath); statErr == nil {
			leftovers = append(leftovers, remoteFilePath)
		}
		cleanup()
		return 0, errors.New(msg(envTransfer, transferSudoFailed, err.Error())), leftovers
	}
	cleanup()
	return n, nil, leftovers
}

// rollback 上传失败后删掉本次已写成功的文件。
//
// 只删清单里列的那些完整路径，不扫目录、不用通配符、不删中间目录：写每个文件之前都查过
// 重名，所以这些路径必然都是本次自己建的。删不掉的路径照实带进报文。
func (c *client) rollback(sftpClient *sftp.Client, writtenPaths []string, useSudo bool) string {
	if len(writtenPaths) == 0 {
		return ""
	}
	remove := func(p string) error {
		if useSudo {
			return c.exec(fmt.Sprintf("sudo rm -f '%s'", escapeShellArg(p)))
		}
		return sftpClient.Remove(p)
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
	return rollbackMessage(len(writtenPaths), removed, left)
}

// randomHex 8 位随机 hex（临时目录名）。
func randomHex() string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%x", b)
}
