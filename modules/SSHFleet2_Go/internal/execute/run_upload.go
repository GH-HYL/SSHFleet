package execute

// 上传：把采集好的那张表写到目标目录。
//
// 两条硬规矩（ADR-0022）：
//   - 远端重名不覆盖：开传之前整批问一次，命中就零写入、整批计失败（目标机上那个同名文件
//     是别人先放的，回滚一定不能碰它——现在根本没有需要回滚的东西）；
//   - 中途失败把**本次写成功**的那些删掉（只删清单里的路径，不扫目录、不用通配符）。
//
// `--sudo` 与不 sudo 走同一条写路径：提权时整条 SFTP 会话以 root 跑，直接写最终路径，
// 没有暂存目录这一步（ADR-0063 决定 1）。
//
// 明细进 `terminal-output.txt`，不上屏——传输类的结果由进度面板呈现（老版同一判据）。

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"time"

	"sshfleet2/internal/contract"
	"sshfleet2/internal/message"
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

	remotePath := p.Task.Path
	useSudo := p.Run.Sudo && c.node.User != "root"

	// 这一台的传输预算：起点是入口，枚举（上传没有）、预检、逐文件写都在里面（ADR-0063 决定 6）。
	clock := newTransferClock(ctx, seconds(p.Run.TimeoutTransfer), seconds(p.Run.TimeoutConnect))

	// 明细行随早退一起走：被跳过的软链接在任何一种收场里都要看得见。
	lines := make([]string, 0, len(task.Skipped)+len(files))
	for _, s := range task.Skipped {
		lines = append(lines, message.TransferSkipped(s))
	}
	acct := newTransferAccount(len(files), lines)

	sftpClient, err := c.openSFTP(useSudo)
	if err != nil {
		res.ErrorText = appendText(res.ErrorText, openFailureText(err))
		acct.allFailed()
		acct.settle(res)
		return
	}
	defer func() { _ = sftpClient.Close() }()
	// 到点的动作：关掉这一刻占着连接的那条通道。只关 SFTP，不关整条 SSH 连接——连接还要
	// 留给清理（ADR-0063 决定 6）。
	closeSFTP := func() { _ = sftpClient.Close() }

	var totalBytes int64
	for _, f := range files {
		totalBytes += f.Size
	}

	// 远程目标路径检查：必须存在且是目录（工具不建 -p 指的目录）。
	var fi os.FileInfo
	if end := clock.arm(closeSFTP, func() { fi, err = sftpClient.Stat(remotePath) }); !end.ran() {
		c.uploadStopped(clock, res, acct, nil, nil, useSudo)
		return
	}
	if err != nil {
		// 措辞与下载侧同名——分类按报错原文匹配关键字，两处各写一套就会有一条永远归不了类。
		res.ErrorText = appendText(res.ErrorText, msg(envTransfer, transferRemoteMissing, "remote path not found: "+remotePath))
		acct.allFailed()
		acct.settle(res)
		return
	}
	if !fi.IsDir() {
		res.ErrorText = appendText(res.ErrorText, msg(envTransfer, transferRemoteNotDir, "remote target is not a directory: "+remotePath))
		acct.allFailed()
		acct.settle(res)
		return
	}

	// 本地文件预检：本地读不了是更近、更确定的原因，不该被「磁盘满」抢先报出来。
	for _, f := range files {
		if _, err := os.Stat(f.Path); err != nil {
			acct.allFailed()
			acct.settle(res)
			res.ErrorText = appendText(res.ErrorText, msg(envTransfer, transferLocalUnreadable, fmt.Sprintf("%s: %v", f.Rel, err)))
			return
		}
	}

	// 空间预检：装不下就停在写第一个文件之前，目标机上不会有任何本次的东西。
	// 只有目标目录这一侧——提权时文件直接写最终路径，没有暂存分区要判（ADR-0026）。
	var spaceMsg string
	if end := clock.arm(closeSFTP, func() {
		spaceMsg = spaceShort(sftpProbe(sftpClient), remotePath, useSudo, totalBytes, true)
	}); !end.ran() {
		c.uploadStopped(clock, res, acct, nil, nil, useSudo)
		return
	}
	if spaceMsg != "" {
		res.ErrorText = appendText(res.ErrorText, spaceMsg)
		acct.allFailed()
		acct.settle(res)
		return
	}

	// 远端重名：开传之前整批问一次。命中即零写入——一个字节都不传，所以没有回滚这回事。
	remotePaths := make([]string, 0, len(files))
	for _, f := range files {
		remotePaths = append(remotePaths, path.Join(remotePath, f.Rel))
	}
	existing, end := c.remoteExisting(clock, sftpClient, remotePaths, useSudo)
	if !end.ran() {
		c.uploadStopped(clock, res, acct, nil, nil, useSudo)
		return
	}
	if len(existing) > 0 {
		acct.allFailed()
		acct.settle(res)
		res.ErrorText = appendText(res.ErrorText, msg(envTransfer, transferRemoteExists,
			"file already exists on the remote host: "+remoteNames(existing, remotePath)))
		return
	}

	// 本次已写成功的远端完整路径。失败回滚只删这些——不扫目录、不用通配符。
	written := make([]string, 0, len(files))
	var pacer progressPacer

	for idx, f := range files {
		if ctx.Err() != nil {
			// 被中断同样算「这次没传完」：已经写成功的照样回滚，不能因为「是用户按的
			// 中断」就把残留留在目标机上。
			res.Cancelled = true
			acct.settle(res)
			res.ErrorText = appendText(res.ErrorText, c.rollback(clock, written, useSudo))
			res.ErrorText = appendText(res.ErrorText, abortMessage(true))
			return
		}

		remoteFilePath := path.Join(remotePath, f.Rel)

		localInfo, statErr := os.Stat(f.Path)
		if statErr != nil {
			acct.failed(1)
			acct.line(message.TransferFailure(contract.ModeUpload, f.Rel, statErr))
			res.ErrorText = appendText(res.ErrorText, msg(envTransfer, transferLocalUnreadable, fmt.Sprintf("%s: %v", f.Rel, statErr)))
			res.ErrorText = appendText(res.ErrorText, c.rollback(clock, written, useSudo))
			break
		}
		perm := localInfo.Mode().Perm()

		var (
			writtenBytes int64
			uploadErr    error
			halfWrites   []string
			spaceNote    string
		)
		// 单个文件失败只重传它自己：已传好的文件一个字节都不重发（ADR-0063 决定 9）。
		// putFile 每次重来之前都会删掉它自己的半成品，所以这里只管再调一次。
		attempts := 1 + p.Run.TransferRetry
		for attempt := 1; attempt <= attempts; attempt++ {
			// 每个文件的「写 + 写完核大小」是一次发起：到点就关掉这条通道、等它退出来。
			end := clock.arm(closeSFTP, func() {
				writtenBytes, uploadErr, halfWrites = c.putFile(sftpClient, f.Path, remoteFilePath, perm, idx, totalBytes, len(files), onProg)
			})
			if !end.ran() {
				// armStuck：那次写还没退出来，putFile 没机会把它的尾巴交上来，而目标机上
				// 这个名字已经被 Create 建出来了——算残留，宁可报全（ADR-0022 决定 1）。
				if end == armStuck {
					halfWrites = append(halfWrites, remoteFilePath)
				}
				c.uploadStopped(clock, res, acct, written, halfWrites, useSudo)
				return
			}
			if uploadErr == nil {
				break
			}
			// 失败后再查一次空间：满不是唯一的原因，只有探出来确实不够才改口。
			// **必须在回滚之前问**：回滚会把空间还回去，那时再探永远是「够」。
			// 它同时决定要不要重传——目标盘装不下，重试只是把同一个结果再等一遍。
			if end := clock.arm(closeSFTP, func() {
				spaceNote = spaceShort(sftpProbe(sftpClient), remotePath, useSudo, totalBytes-acct.stat.TotalBytes, false)
			}); !end.ran() {
				c.uploadStopped(clock, res, acct, written, halfWrites, useSudo)
				return
			}
			if attempt == attempts || spaceNote != "" || !retryableTransferError(uploadErr) {
				break
			}
		}

		if uploadErr != nil {
			acct.failed(1)
			acct.line(message.TransferFailure(contract.ModeUpload, f.Rel, uploadErr))
			reason := uploadErr.Error()
			if !isMessage(reason) {
				reason = msg(envTransfer, transferRemoteFileFailed, reason)
			}
			res.ErrorText = appendText(res.ErrorText, fmt.Sprintf("%s: %s", f.Rel, reason))
			res.ErrorText = appendText(res.ErrorText, spaceNote)
			res.ErrorText = appendText(res.ErrorText, c.rollback(clock, written, useSudo))
			// 半成品没删掉时照实说：它们不在回滚清单里，混进回滚计数会失真。
			res.ErrorText = appendText(res.ErrorText, partialLeft(halfWrites))
			break
		}
		acct.wrote(writtenBytes)
		// 只有**写成功**的才进清单——回滚删的就是这份清单，多记一条就多一分误删风险。
		written = append(written, remoteFilePath)

		// 逐文件进展也报上去：文件小、字节级回调太稀时，面板靠它动。
		if pacer.allow() {
			reportTransfer(onProg, idx, c.node.IP, acct.stat.TotalBytes, totalBytes, len(files),
				acct.stat.SuccessFiles, acct.stat.FailFiles)
		}
	}

	acct.settle(res)
}

// uploadStopped 传输预算到点时的收场：这一台的所有文件一起作废，照实说目标机上还剩什么。
//
// 先清理再报（ADR-0065）：本次写成功的与当前写到一半的一起删，删不掉的照实列出来。
// 清理只看自己那份收尾预算，超时与中断都停不下它。
//
// written／half 由上传自己交进来：它们是「目标机上现在可能有什么」，下载用不上，
// 所以不进那本共用的账。
func (c *client) uploadStopped(clock *transferClock, res *contract.NodeResult, acct *transferAccount, written, half []string, useSudo bool) {
	acct.failedRest()
	acct.settle(res)

	left := dedupePaths(written, half)

	// 到点那一刻父 ctx 已取消：记的是中断，不是这一台的毛病（两者互斥，与命令模式同口径）。
	// 清理照做——中断不是清理的停止条件。
	if clock.interrupted() {
		res.Cancelled = true
		removed, remained, _ := c.cleanupWritten(clock, left, useSudo)
		res.ErrorText = appendText(res.ErrorText, rollbackMessage(len(left), removed, remained))
		res.ErrorText = appendText(res.ErrorText, abortMessage(true))
		return
	}

	res.TimedOut = true
	secs := int(clock.transfer.Seconds())
	if len(left) == 0 {
		res.ErrorText = appendText(res.ErrorText, timeoutNoLeft(secs))
		return
	}

	removed, remained, attempted := c.cleanupWritten(clock, left, useSudo)
	switch {
	case !attempted:
		res.ErrorText = appendText(res.ErrorText, timeoutLeftNotAttempted(remained))
	case len(remained) == 0:
		// 两句各说一件事：目标机现在没有本次的东西，以及删掉了几个。
		res.ErrorText = appendText(res.ErrorText, timeoutNoLeft(secs))
		res.ErrorText = appendText(res.ErrorText, rollbackMessage(len(left), removed, nil))
	default:
		res.ErrorText = appendText(res.ErrorText, timeoutLeft(removed, len(left), remained))
	}
}

// putFile 经 SFTP 写一个文件，返回写入字节数与没删掉的半成品。
//
// 两种身份共用这一条：提权时底层会话已是 root，中间目录同样由 SFTP 自己建。
func (c *client) putFile(sftpClient *sftpConn, localPath, remoteFilePath string, perm os.FileMode, seq int, totalBytes int64, totalFiles int, onProg onProgressN) (int64, error, []string) {
	// 中间目录按需创建：上传目录的相对层级要保留。目标目录（-p）本身仍必须已存在。
	if dir := path.Dir(remoteFilePath); dir != "." {
		if err := sftpClient.MkdirAll(dir); err != nil {
			return 0, errors.New(msg(envTransfer, transferRemoteFileFailed, fmt.Sprintf("create remote dir %s: %v", dir, err))), nil
		}
	}

	src, err := os.Open(localPath)
	if err != nil {
		return 0, noRetry(errors.New(msg(envTransfer, transferLocalUnreadable, err.Error()))), nil
	}
	defer func() { _ = src.Close() }()
	// 本机这一份多大：写完要拿它跟目标机对（ADR-0063 决定 11）。
	info, err := src.Stat()
	if err != nil {
		return 0, noRetry(errors.New(msg(envTransfer, transferLocalUnreadable, err.Error()))), nil
	}

	dst, err := sftpClient.Create(remoteFilePath)
	if err != nil {
		// 原始错误要留着：没有权限、文件不存在这类状态码决定要不要重试。
		return 0, transferFail(transferRemoteFileFailed, err), nil
	}

	n, copyErr := copyStream(sending, dst, src, onProg, seq, c.node.IP, totalBytes, totalFiles)
	// 先关再问大小：没关掉之前，目标机那边的写入可能还挂在缓冲里。
	closeErr := dst.Close()

	if copyErr != nil {
		return 0, transferFail(transferRemoteFileFailed, copyErr), leftoverOf(sftpClient, remoteFilePath)
	}
	if closeErr != nil {
		return 0, transferFail(transferRemoteFileFailed, closeErr), leftoverOf(sftpClient, remoteFilePath)
	}

	// 写完核大小：只比字节数，不比内容（ADR-0063 决定 11 的取舍）。
	remoteInfo, err := sftpClient.Stat(remoteFilePath)
	if err != nil {
		return 0, transferFail(transferRemoteFileFailed, fmt.Errorf("stat after write: %w", err)), leftoverOf(sftpClient, remoteFilePath)
	}
	if remoteInfo.Size() != info.Size() {
		// 删掉它自己：留着会让这次重传撞「远程文件已存在」。
		return 0, errors.New(sizeMismatchMessage(info.Size(), remoteInfo.Size())), leftoverOf(sftpClient, remoteFilePath)
	}

	// 权限设置失败不阻断（老版同口径）。
	_ = sftpClient.Chmod(remoteFilePath, perm)
	return n, nil, nil
}

// leftoverOf 删掉本次写坏的那个远端文件；删不掉就把它交回去，照实写进报文。
func leftoverOf(sftpClient *sftpConn, remoteFilePath string) []string {
	if err := sftpClient.Remove(remoteFilePath); err != nil {
		return []string{remoteFilePath}
	}
	return nil
}

// rollback 上传失败后删掉本次已写成功的文件。
//
// 只删清单里列的那些完整路径，不扫目录、不用通配符、不删中间目录：写每个文件之前都查过
// 重名，所以这些路径必然都是本次自己建的。删不掉的路径照实带进报文。
//
// 删法与超时收尾共用同一个动作（ADR-0065）：同一条 SSH 连接上另开命令会话、一条远端循环
// 删完，只受自己那份收尾预算约束——一次失败发生在传输快用完时，逐条删会一条条卡在死链上
// 永远回不来，而两边口径不一致还会让「删到一半被自己的超时打断」变成假残留。
func (c *client) rollback(clock *transferClock, writtenPaths []string, useSudo bool) string {
	if len(writtenPaths) == 0 {
		return ""
	}
	removed, left, _ := c.cleanupWritten(clock, writtenPaths, useSudo)
	return rollbackMessage(len(writtenPaths), removed, left)
}
