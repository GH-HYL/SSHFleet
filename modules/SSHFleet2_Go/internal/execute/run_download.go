package execute

// 下载：按远端路径把文件收回来。
//
// 规矩与老版一致：
//   - 目录按 IP 建子目录、保留远端相对层级；
//   - 软链接**过滤但列出**（不静默丢弃）；
//   - 名字一律按**远端**规则切，本机落不下就整批计失败（不改名、不换目录）；
//   - 写到一半失败的本地半成品删掉；下载不做回滚（没成功的不算本次成果）。

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"sshfleet2/internal/contract"
	"sshfleet2/internal/criterion"
	"sshfleet2/internal/message"
)

// download 一台的下载。
func (c *client) download(ctx context.Context, p *contract.Plan, res *contract.NodeResult, onProg onProgressN) {
	// 与上传同一处口径：从开工到收场这一段，失败也算（屏幕明细与执行期日志都读它）。
	start := time.Now()
	defer func() { res.Cost = time.Since(start) }()

	remotePath := p.Task.Download.RemotePath
	localPath := p.Task.Path
	useSudo := p.Run.Sudo && c.node.User != "root"

	// 这一台的传输预算：起点是入口，枚举、预检、逐文件取都在里面（ADR-0063 决定 6）。
	clock := newTransferClock(ctx, seconds(p.Run.TimeoutTransfer), seconds(p.Run.TimeoutConnect))

	// 早退时也要记账（与上传同口径）：这几格在枚举之前就声明，任何一条 return
	// 都能交出一份「这次打算传几个、成了几个」。
	var (
		files      []fileEntry
		symlinks   []string // 被过滤的软链接（相对路径），只用于在结果里提示
		totalBytes int64
	)

	sftpClient, err := c.openSFTP(useSudo)
	if err != nil {
		res.ErrorText = appendText(res.ErrorText, openFailureText(err))
		failTransfer(res, files)
		return
	}
	defer func() { _ = sftpClient.Close() }()
	// 到点的动作：关掉这一刻占着连接的那条通道（ADR-0065）。远端零写入，所以没有清理要留。
	closeSFTP := func() { _ = sftpClient.Close() }

	// 远程路径预检（sudo 时用 sudo test）；路径转义规则与 find/mv 同源。
	checkCmd := fmt.Sprintf("test -e '%s'", criterion.EscapeSingle(remotePath))
	if useSudo {
		checkCmd = fmt.Sprintf("sudo test -e '%s'", criterion.EscapeSingle(remotePath))
	}
	if err, end := c.execArmed(clock, checkCmd); !end.ran() {
		c.downloadStopped(clock, res, files, 0, 0, nil)
		return
	} else if err != nil {
		// 预检查是中间步骤：它自己的退出码进 Steps，不顶成结果退出码。
		stepsOf(res, contract.StepPrecheck, err)
		res.ErrorText = appendText(res.ErrorText, msg(envTransfer, transferRemoteMissing, "remote path not found: "+remotePath))
		failTransfer(res, files)
		return
	}
	stepsOf(res, contract.StepPrecheck, nil)

	var fi os.FileInfo
	if end := clock.arm(closeSFTP, func() { fi, err = sftpClient.Stat(remotePath) }); !end.ran() {
		c.downloadStopped(clock, res, files, 0, 0, nil)
		return
	}
	if err != nil {
		res.ErrorText = appendText(res.ErrorText, msg(envTransfer, transferRemoteUnreachable, "remote path not accessible: "+remotePath))
		failTransfer(res, files)
		return
	}

	// 远端路径本身即软链接：过滤（不下载、不当失败）。
	rootIsLink := false
	if lfi, lerr := sftpClient.Lstat(remotePath); lerr == nil && lfi.Mode()&os.ModeSymlink != 0 {
		rootIsLink = true
		// 名字按**远端**规则切：这条路径是远端的，`\` 在那边只是一个普通字符。
		symlinks = append(symlinks, path.Base(remotePath))
	}

	switch {
	case rootIsLink:
		// 已过滤，落到下面的「无可下载文件」判定。

	case fi.IsDir():
		// 一次 find 同时枚举真文件（F）与软链接（L）：软链接过滤但要在结果里提示。
		// 记录按 NUL 切，名字里带换行不会被拆开（ADR-0063 决定 4）。
		runFind := func(cmd string) (string, armEnd, error) { return c.captureArmed(clock, cmd) }
		enumFiles, enumLinks, end, err := enumerateRemote(sftpClient, remotePath, useSudo, runFind)
		if !end.ran() {
			c.downloadStopped(clock, res, files, 0, 0, nil)
			return
		}
		if err != nil {
			stepsOf(res, contract.StepEnumerate, err)
			res.ErrorText = appendText(res.ErrorText, msg(envTransfer, transferListFailed, err.Error()))
			failTransfer(res, files)
			return
		}
		stepsOf(res, contract.StepEnumerate, nil)
		files = append(files, enumFiles...)
		symlinks = append(symlinks, enumLinks...)
		for _, f := range files {
			totalBytes += f.Size
		}

	default:
		// 单文件：名字按远端规则切。
		files = append(files, fileEntry{Rel: path.Base(remotePath), Size: fi.Size()})
		totalBytes = fi.Size()
	}

	sort.Strings(symlinks)

	totalFiles := len(files)
	if totalFiles == 0 {
		// 过滤后没有任何可下载文件才报错（全是链接与目录本就为空，措辞分开）。
		if len(symlinks) > 0 {
			res.ErrorText = appendText(res.ErrorText, msg(envTransfer, transferNoFiles,
				fmt.Sprintf("all %d entries are symlinks (filtered)", len(symlinks))))
		} else {
			res.ErrorText = appendText(res.ErrorText, msg(envTransfer, transferNoFiles, "no files to download under "+remotePath))
		}
		return
	}

	// 本机重名：开传之前整批查（ADR-0063 决定 7）。查在落地目录之前——拦下时连目录都不建。
	ipDir := filepath.Join(localPath, c.node.IP)
	if conflicts := localConflicts(ipDir, files, fi.IsDir(), remotePath); len(conflicts) > 0 {
		res.ErrorText = appendText(res.ErrorText, msg(envTransfer, transferLocalExists,
			"already exists locally: "+strings.Join(conflicts, ", ")))
		failTransfer(res, files)
		return
	}

	// 本地落地目录：开工后才建。参数阶段只确认过 -p 存在，从确认屏到真正开传之间它可能
	// 已被删、被改权限——这类失败必须落到「本地文件错误」，不能因为「前面查过了」就不报。
	if err := os.MkdirAll(ipDir, 0o755); err != nil {
		res.ErrorText = appendText(res.ErrorText, msg(envTransfer, transferLocalDirFailed, err.Error()))
		failTransfer(res, files)
		return
	}

	success, failed := 0, 0
	var downloadedBytes int64
	lines := make([]string, 0, len(symlinks)+len(files))
	for _, s := range symlinks {
		lines = append(lines, message.TransferSkipped(s))
	}

	for i, file := range files {
		if ctx.Err() != nil {
			res.Cancelled = true
			res.Transfer.SuccessFiles, res.Transfer.FailFiles = success, failed
			res.Transfer.TotalBytes, res.Transfer.TotalFiles = downloadedBytes, totalFiles
			res.Output = buildTransferOutput(totalFiles, success, failed, lines)
			res.ErrorText = appendText(res.ErrorText, abortMessage(true))
			return
		}

		// 名字一律取自远端、按**远端**规则切；落到本机时用的就是这个原名。
		var remoteFilePath, localFilePath, badName string
		if fi.IsDir() {
			remoteFilePath = remotePath + "/" + file.Rel
			localFilePath = filepath.Join(ipDir, file.Rel)
			if !localRelUsable(file.Rel) {
				badName = file.Rel
			}
		} else {
			name := path.Base(remotePath)
			if !localRelUsable(name) {
				badName = name
			}
			remoteFilePath = remotePath
			localFilePath = filepath.Join(ipDir, name)
		}
		if badName != "" {
			// 悄悄改名落盘会让人以为远端那个文件叫这名；多建一层目录更糟。
			// 一个错误就全部终止：同批剩下的（含当前这个）一并计失败，否则 total 与
			// 「成功 + 失败」对不上。
			failed += len(files) - i
			reason := msg(envTransfer, transferLocalFileFailed, "name not usable as a local file name: "+badName)
			lines = append(lines, message.TransferFailure(contract.ModeDownload, badName, reason))
			res.ErrorText = appendText(res.ErrorText, fmt.Sprintf("%s: %s", badName, reason))
			break
		}

		if err := os.MkdirAll(filepath.Dir(localFilePath), 0o755); err != nil {
			failed += len(files) - i
			reason := msg(envTransfer, transferLocalDirFailed, err.Error())
			lines = append(lines, message.TransferFailure(contract.ModeDownload, file.Rel, reason))
			res.ErrorText = appendText(res.ErrorText, fmt.Sprintf("%s: %s", file.Rel, reason))
			break
		}

		var (
			n           int64
			downloadErr error
			halfWrites  []string
		)
		// 单个文件失败只重下它自己（ADR-0063 决定 9）：getFile 每次重来前会删掉本地半成品。
		attempts := 1 + p.Run.TransferRetry
		for attempt := 1; attempt <= attempts; attempt++ {
			// 每个文件的「取 + 写完核大小」是一次发起：到点就关掉这条通道、等它退出来。
			end := clock.arm(closeSFTP, func() {
				n, downloadErr, halfWrites = getFile(sftpClient, remoteFilePath, localFilePath, i, c.node.IP, totalBytes, totalFiles, file.Size, onProg)
			})
			if !end.ran() {
				c.downloadStopped(clock, res, files, success, downloadedBytes, lines)
				return
			}
			if downloadErr == nil || attempt == attempts || !retryableTransferError(downloadErr) {
				break
			}
		}
		if downloadErr != nil {
			failed++
			// 「下载失败」只留在输出明细行；报错原文格放报文。
			lines = append(lines, message.TransferFailure(contract.ModeDownload, file.Rel, downloadErr))
			reason := downloadErr.Error()
			if !isMessage(reason) {
				reason = msg(envTransfer, transferLocalFileFailed, reason)
			}
			res.ErrorText = appendText(res.ErrorText, fmt.Sprintf("%s: %s", file.Rel, reason))
			res.ErrorText = appendText(res.ErrorText, localPartialLeft(halfWrites))
			break
		}
		success++
		downloadedBytes += n
	}

	res.Output = buildTransferOutput(totalFiles, success, failed, lines)
	// 传输不是命令执行：不写任何退出码；有成有败由结果判定按四个数给。
	res.Transfer.SuccessFiles = success
	res.Transfer.FailFiles = failed
	res.Transfer.TotalBytes = downloadedBytes
	res.Transfer.TotalFiles = totalFiles
}

// downloadStopped 传输预算到点时的收场。
//
// 远端零写入，所以没有残留清单这回事（ADR-0022 决定 5 不变）：本机那个写到一半的文件由
// getFile 自己删掉，这里只报超时与四个数。
func (c *client) downloadStopped(clock *transferClock, res *contract.NodeResult, files []fileEntry, success int, bytes int64, lines []string) {
	failed := len(files) - success
	res.Transfer.SuccessFiles = success
	res.Transfer.FailFiles = failed
	res.Transfer.TotalBytes = bytes
	res.Transfer.TotalFiles = len(files)
	res.Output = buildTransferOutput(len(files), success, failed, lines)

	// 到点那一刻父 ctx 已取消：记的是中断，不是这一台的毛病（两者互斥）。
	if clock.interrupted() {
		res.Cancelled = true
		res.ErrorText = appendText(res.ErrorText, abortMessage(true))
		return
	}
	res.TimedOut = true
	res.ErrorText = appendText(res.ErrorText, timeoutNoLeft(int(clock.transfer.Seconds())))
}

// getFile 经 SFTP 取一个文件，返回下载字节数；失败删本地半成品。
func getFile(sftpClient *sftpConn, remoteFilePath, localFilePath string, seq int, ip string, totalBytes int64, totalFiles int, wantSize int64, onProg onProgressN) (int64, error, []string) {
	// 打开的是**远端**文件：这一步失败是目标机那边的事（文件刚被删、权限不够），
	// 挂 remote_file_failed——挂 local_file_failed 会让「本机写不进去」与「远端读不出来」
	// 共用同一个环节名，按分类排查时会找错方向。
	src, err := sftpClient.Open(remoteFilePath)
	if err != nil {
		return 0, errors.New(msg(envTransfer, transferRemoteFileFailed, err.Error())), nil
	}
	defer func() { _ = src.Close() }()

	dst, err := os.Create(localFilePath)
	if err != nil {
		// 本机建不出文件：重试还是建不出，别等第二遍。
		return 0, noRetry(errors.New(msg(envTransfer, transferLocalFileFailed, err.Error()))), nil
	}

	n, copyErr := copyDownload(dst, src, onProg, seq, ip, totalBytes, totalFiles)
	// 先关再问大小：没关掉之前，本机这边的写入可能还挂在缓冲里。
	closeErr := dst.Close()

	if copyErr != nil {
		return 0, transferFail(transferLocalFileFailed, copyErr), removeLocal(localFilePath)
	}
	if closeErr != nil {
		return 0, transferFail(transferLocalFileFailed, closeErr), removeLocal(localFilePath)
	}

	// 写完核大小：与枚举时远端给的那个数比（ADR-0063 决定 11）。
	info, err := os.Stat(localFilePath)
	if err != nil {
		return 0, transferFail(transferLocalFileFailed, err), removeLocal(localFilePath)
	}
	if info.Size() != wantSize {
		return 0, errors.New(sizeMismatchMessage(info.Size(), wantSize)), removeLocal(localFilePath)
	}
	return n, nil, nil
}

// removeLocal 删掉本机这个不合格的落盘文件；删不掉就交回去照实说。
func removeLocal(localFilePath string) []string {
	if err := os.Remove(localFilePath); err != nil {
		// 本机半成品没删掉：下次重试会撞上这个残缺文件。
		return []string{localFilePath}
	}
	return nil
}

// localConflicts 本机已经占了名字的那些落点（相对路径，按枚举顺序）。
//
// 用 `Lstat` 不用 `Stat`：断掉的符号链接也算「名字被占用」，而 `Stat` 会当它不存在，
// 紧接着的 `os.Create` 就顺着链接写到别处去了。
//
// 名字在本机落不下的跳过这一查：它们会在逐文件那一段按「本机文件名不可用」报出来，
// 两条判据不重叠。
func localConflicts(ipDir string, files []fileEntry, isDir bool, remotePath string) []string {
	var hit []string
	for _, f := range files {
		name := f.Rel
		if !isDir {
			name = path.Base(remotePath)
		}
		if !localRelUsable(name) {
			continue
		}
		if _, err := os.Lstat(filepath.Join(ipDir, name)); err == nil {
			hit = append(hit, name)
		}
	}
	return hit
}

// localRelUsable 远端给出的名字能不能在本机原样落盘。
//
// 逐段检查：`rel` 是远端枚举出来的相对路径（段间是 POSIX 的 `/`），每一段都要是**一个**
// 可用的本机文件名——含本机分隔符（Windows 上 `a\b.log`）就不行，`filepath.Base` 切完会
// 变短；保留设备名（NUL 之类）由 `IsLocal` 挡。
func localRelUsable(rel string) bool {
	for _, elem := range strings.Split(rel, "/") {
		if elem == "" || elem == "." || elem == ".." {
			return false
		}
		if filepath.Base(elem) != elem || !filepath.IsLocal(elem) {
			return false
		}
	}
	return true
}
