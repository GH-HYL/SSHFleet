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

	"github.com/pkg/sftp"

	"sshfleet2/internal/contract"
)

// download 一台的下载。
func (c *client) download(ctx context.Context, p *contract.Plan, res *contract.NodeResult, onProg onProgressN) {
	remotePath := p.Task.Download.RemotePath
	localPath := p.Task.Path
	useSudo := p.Run.Sudo && c.node.User != "root"

	sftpClient, err := sftp.NewClient(c.conn)
	if err != nil {
		res.ErrorText = appendText(res.ErrorText, msg(envTransfer, transferClientFailed, err.Error()))
		return
	}
	defer func() { _ = sftpClient.Close() }()

	// 远程路径预检（sudo 时用 sudo test）；路径转义规则与 find/mv 同源。
	checkCmd := fmt.Sprintf("test -e '%s'", escapeShellArg(remotePath))
	if useSudo {
		checkCmd = fmt.Sprintf("sudo test -e '%s'", escapeShellArg(remotePath))
	}
	if err := c.exec(checkCmd); err != nil {
		// 预检查是中间步骤：它自己的退出码进 Steps，不顶成结果退出码。
		res.Steps = append(res.Steps, contract.Step{Name: contract.StepPrecheck, ExitCode: extractExitCode(err)})
		res.ErrorText = appendText(res.ErrorText, msg(envTransfer, transferRemoteMissing, "remote path not found: "+remotePath))
		return
	}

	fi, err := sftpClient.Stat(remotePath)
	if err != nil {
		res.ErrorText = appendText(res.ErrorText, msg(envTransfer, transferRemoteUnreachable, "remote path not accessible: "+remotePath))
		return
	}

	var (
		files      []fileEntry
		symlinks   []string // 被过滤的软链接（相对路径），只用于在结果里提示
		totalBytes int64
	)

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
		findCmd := fmt.Sprintf("find '%s' \\( -type f -printf 'F %%s %%P\\n' \\) -o \\( -type l -printf 'L %%P\\n' \\)",
			escapeShellArg(remotePath))
		if useSudo {
			findCmd = "sudo " + findCmd
		}
		output, err := c.execCapture(findCmd)
		if err != nil {
			res.Steps = append(res.Steps, contract.Step{Name: contract.StepEnumerate, ExitCode: extractExitCode(err)})
			res.ErrorText = appendText(res.ErrorText, msg(envTransfer, transferListFailed, err.Error()))
			return
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
				files = append(files, fileEntry{Rel: parts[1], Size: size})
				totalBytes += size
			case strings.HasPrefix(line, "L "):
				symlinks = append(symlinks, line[2:])
			}
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

	// 本地落地目录：开工后才建。参数阶段只确认过 -p 存在，从确认屏到真正开传之间它可能
	// 已被删、被改权限——这类失败必须落到「本地文件错误」，不能因为「前面查过了」就不报。
	ipDir := filepath.Join(localPath, c.node.IP)
	if err := os.MkdirAll(ipDir, 0o755); err != nil {
		res.ErrorText = appendText(res.ErrorText, msg(envTransfer, transferLocalDirFailed, err.Error()))
		return
	}

	success, failed := 0, 0
	var downloadedBytes int64
	lines := make([]string, 0, len(symlinks)+len(files))
	for _, s := range symlinks {
		lines = append(lines, fmt.Sprintf("%s: 已跳过（符号链接）", s))
	}

	for i, file := range files {
		if ctx.Err() != nil {
			res.Cancelled = true
			res.Transfer.SuccessFiles, res.Transfer.FailFiles = success, failed
			res.Transfer.TotalBytes, res.Transfer.TotalFiles = downloadedBytes, totalFiles
			res.Output = buildTransferOutput(totalFiles, success, failed, lines)
			res.ErrorText = appendText(res.ErrorText, msg(envAbort, abortCanceled, "interrupted after session established"))
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
			lines = append(lines, fmt.Sprintf("%s: 下载失败 - %s", badName, reason))
			res.ErrorText = appendText(res.ErrorText, fmt.Sprintf("%s: %s", badName, reason))
			break
		}

		if err := os.MkdirAll(filepath.Dir(localFilePath), 0o755); err != nil {
			failed += len(files) - i
			reason := msg(envTransfer, transferLocalDirFailed, err.Error())
			lines = append(lines, fmt.Sprintf("%s: 下载失败 - %s", file.Rel, reason))
			res.ErrorText = appendText(res.ErrorText, fmt.Sprintf("%s: %s", file.Rel, reason))
			break
		}

		n, downloadErr, halfWrites := getFile(sftpClient, remoteFilePath, localFilePath, i, c.node.IP, totalBytes, totalFiles, onProg)
		if downloadErr != nil {
			failed++
			// 「下载失败」只留在输出明细行；报错原文格放报文。
			lines = append(lines, fmt.Sprintf("%s: 下载失败 - %v", file.Rel, downloadErr))
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

// getFile 经 SFTP 取一个文件，返回下载字节数；失败删本地半成品。
func getFile(sftpClient *sftp.Client, remoteFilePath, localFilePath string, seq int, ip string, totalBytes int64, totalFiles int, onProg onProgressN) (int64, error, []string) {
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
		return 0, errors.New(msg(envTransfer, transferLocalFileFailed, err.Error())), nil
	}
	defer func() { _ = dst.Close() }()

	n, err := copyDownload(dst, src, onProg, seq, ip, totalBytes, totalFiles)
	if err != nil {
		var leftovers []string
		if rmErr := os.Remove(localFilePath); rmErr != nil {
			// 本机半成品没删掉：下次重试会撞上这个残缺文件。
			leftovers = append(leftovers, localFilePath)
		}
		return 0, errors.New(msg(envTransfer, transferLocalFileFailed, err.Error())), leftovers
	}
	return n, nil, nil
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
