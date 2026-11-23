package execute

// 传输（上传与下载）共用的那几件：环节名、报文的拼法、空间预检、回滚陈述。
//
// 环节名与报文格式照老版 `ssh/stages.go`：判定层按这些字符串认分类，改名等于让那一类
// 失败落到「未归类」。写法纪律同报文那一条——只陈述「哪一步、什么现象」，不下结论。

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/pkg/sftp"
)

// 传输一摊的环节名。
const (
	transferClientFailed      = "client_failed"      // SFTP 子系统建不起来
	transferRemoteMissing     = "remote_missing"     // 远程路径在目标机上不存在
	transferRemoteNotDir      = "remote_not_dir"     // 上传目标不是目录
	transferRemoteUnreachable = "remote_unreachable" // 远程路径访问不了
	transferListFailed        = "list_failed"        // 枚举远程文件失败
	transferNoFiles           = "no_files"           // 路径下没有可传的文件
	transferLocalUnreadable   = "local_unreadable"   // 本地文件不存在或读不了
	transferLocalDirFailed    = "local_dir_failed"   // 本机建落地目录失败
	transferRemoteFileFailed  = "remote_file_failed" // 远端建目录 / 建文件 / 写文件失败
	transferLocalFileFailed   = "local_file_failed"  // 下载侧建本地文件 / 写本地失败
	transferRemoteExists      = "remote_exists"      // 目标机上已有同名文件
	transferLocalExists       = "local_exists"       // 本机下载目录里已有同名文件
	transferTargetSpaceShort  = "target_space_short" // 目标目录所在分区的可用量不够
	transferSizeMismatch      = "size_mismatch"      // 写完核大小不一致
	transferRolledBack        = "rolled_back"        // 上传失败后，本次写进去的文件已清干净
	transferRollbackPartial   = "rollback_partial"   // 回滚没清干净：还剩哪些路径
	transferPartialLeft       = "partial_left"       // 写到一半的文件没删掉（上传，远端）
	transferLocalPartialLeft  = "local_partial_left" // 写到一半的文件没删掉（下载，本机）
)

// abort 一摊的环节名：被中断。两个方向各一句——连上之前被打断与连上之后被打断，
// 排查时问的不是同一件事。
const (
	abortCanceled        = "canceled"         // 会话建立之后被中断
	abortConnectCanceled = "connect_canceled" // 建连途中被中断
)

// appendText 把一段报文追加进报错原文那一格（空的那段不加）。
//
// 顺序固定为**原文在前、报文在后**：原文是权威原文，报文是对这次收尾的陈述。
// 谁都不覆盖谁——盖掉就是吞信息。
func appendText(existing, msg string) string {
	msg = strings.TrimRight(msg, "\n")
	if msg == "" {
		return existing
	}
	existing = strings.TrimRight(existing, "\n")
	if existing == "" {
		return msg
	}
	return existing + "\n" + msg
}

// transferError 一条传输失败：给使用者看的那句，外加原始错误。
//
// 原始错误要留着：判「重来一次有没有意义」看的是 SFTP 状态码，不是那句话的措辞。
type transferError struct {
	text string
	err  error
}

func (e *transferError) Error() string { return e.text }

// Unwrap 让上层能用 `errors.As` 摸到库给的那个状态错误。
func (e *transferError) Unwrap() error { return e.err }

// transferFail 把一次远端失败编成报文，原始错误挂在里面。
func transferFail(stage string, err error) error {
	return &transferError{text: msg(envTransfer, stage, err.Error()), err: err}
}

// permanentError 标注「重来一次还是同一个结果」的失败：本机读不了、本机建不了文件。
//
// 只影响要不要重试，不影响报文——Error() 原样交回里面那句。
type permanentError struct{ err error }

func (e *permanentError) Error() string { return e.err.Error() }

func (e *permanentError) Unwrap() error { return e.err }

// noRetry 把一条失败标成不必重试。
func noRetry(err error) error { return &permanentError{err: err} }

// sizeMismatchMessage 写完核大小不符那句：两个数，不带结论。
func sizeMismatchMessage(local, remote int64) string {
	return msg(envTransfer, transferSizeMismatch, fmt.Sprintf("local %d bytes, remote %d bytes", local, remote))
}

// retryableTransferError 这个失败重来一次有没有意义（ADR-0063 决定 10）。
//
// 按 SFTP 状态码判，不按文案判：没有权限、文件不存在、操作不支持这三类，重试只是把
// 同一个结果再等一遍。目标盘满认不出状态码（协议 v3 只有笼统的 failure），由上层那次
// 空间探测负责挡掉。
func retryableTransferError(err error) bool {
	var perm *permanentError
	if errors.As(err, &perm) {
		return false
	}
	var status *sftp.StatusError
	if !errors.As(err, &status) {
		return true
	}
	switch status.Code {
	case uint32(sftp.ErrSSHFxNoSuchFile), uint32(sftp.ErrSSHFxPermissionDenied), uint32(sftp.ErrSSHFxOpUnsupported):
		return false
	}
	return true
}

// openFailureText 开传输通道失败时进报错原文的那句。
//
// 提权探测那几种失败自己就是报文（探不到、sudo 拒绝）；库给的原始错误才需要补上环节名。
func openFailureText(err error) string {
	reason := err.Error()
	if isMessage(reason) {
		return reason
	}
	return msg(envTransfer, transferClientFailed, reason)
}

// rollbackMessage 上传回滚之后那句陈述。
//
// 三种情形都说话——删干净了也要说，使用者在下次撞上「远程文件已存在」时要能回头看出
// 上次到底清没清。零个文件被写过时返回空串：那时目标机本来就没留下东西。
func rollbackMessage(written, removed int, left []string) string {
	if written == 0 {
		return ""
	}
	if len(left) == 0 {
		return msg(envTransfer, transferRolledBack,
			fmt.Sprintf("%d of %d written files removed", removed, written))
	}
	return msg(envTransfer, transferRollbackPartial,
		fmt.Sprintf("%d of %d written files removed, left: %s", removed, written, strings.Join(left, ", ")))
}

// partialLeft 上传侧「半成品没删掉」那句。
func partialLeft(paths []string) string {
	if len(paths) == 0 {
		return ""
	}
	return msg(envTransfer, transferPartialLeft, "half-written file left: "+strings.Join(paths, ", "))
}

// localPartialLeft 下载侧的同一条陈述。
func localPartialLeft(paths []string) string {
	if len(paths) == 0 {
		return ""
	}
	return msg(envTransfer, transferLocalPartialLeft, "half-written file left: "+strings.Join(paths, ", "))
}

// —— 空间预检（ADR-0026） ——

// spaceShort 问一次**目标目录所在分区**：装不下就给出该发的那条报文，够就一个字都不说。
//
// 只有这一侧：提权时文件直接写最终路径，没有暂存分区要判（ADR-0063 决定 1）。
// beforeWrite 区分两种场景：预检（一个文件都没写，报文里要声明这一点）与失败后再查
// （已经写过——探测发生在回滚之前，那时文件还在，只说还差多少）。
func spaceShort(probe func(string, bool) (int64, bool), remotePath string, asRoot bool, need int64, beforeWrite bool) string {
	avail, ok := probe(remotePath, asRoot)
	if !ok || avail >= need {
		return ""
	}
	return msg(envTransfer, transferTargetSpaceShort, spaceShortDetail(remotePath, avail, need, beforeWrite))
}

// spaceShortDetail 报文正文：路径 + 两个数（执行侧只陈述事实）。
func spaceShortDetail(p string, avail, need int64, beforeWrite bool) string {
	// 目标目录常以 "/" 结尾（-p /opt/ 那种写法），报文里别写成 "/opt/: 51 MB"。
	if trimmed := strings.TrimRight(p, "/"); trimmed != "" {
		p = trimmed
	}
	if beforeWrite {
		return fmt.Sprintf("%s: %s available, %s needed; no file written",
			p, humanBytes(avail), humanBytes(need))
	}
	return fmt.Sprintf("%s: %s available, %s left to send", p, humanBytes(avail), humanBytes(need))
}

// sftpAvail 问一次某条路径所在分区还能用多少字节；探不出来就返回 false。
//
// 取哪个字段由「谁在写」决定：以 root 写时 root 能用保留块（Bfree），普通账号只能用
// Bavail。块大小按 Frsize 算，有的实现只填 Bsize 时退回它。
func sftpAvail(client *sftpConn, p string, asRoot bool) (int64, bool) {
	st, err := client.StatVFS(p)
	if err != nil {
		return 0, false
	}
	unit := st.Frsize
	if unit == 0 {
		unit = st.Bsize
	}
	blocks := st.Bavail
	if asRoot {
		blocks = st.Bfree
	}
	return int64(blocks) * int64(unit), true
}

// sftpProbe 把这条连接的探测包成 spaceShort 要的函数值。
func sftpProbe(client *sftpConn) func(string, bool) (int64, bool) {
	return func(p string, asRoot bool) (int64, bool) { return sftpAvail(client, p, asRoot) }
}

// humanBytes 报文里的人读大小：按量级选单位，保留两位小数，多余的零不写。
func humanBytes(n int64) string {
	const (
		kb = int64(1024)
		mb = 1024 * kb
		gb = 1024 * mb
	)
	switch {
	case n >= gb:
		return trimFloat(float64(n)/float64(gb)) + " GB"
	case n >= mb:
		return trimFloat(float64(n)/float64(mb)) + " MB"
	case n >= kb:
		return trimFloat(float64(n)/float64(kb)) + " KB"
	default:
		return strconv.FormatInt(n, 10) + " B"
	}
}

// trimFloat 四舍五入到两位小数，并用最短表示输出（"3" 而不是 "3.00"）。
func trimFloat(v float64) string {
	return strconv.FormatFloat(math.Round(v*100)/100, 'f', -1, 64)
}

// buildTransferOutput 传输明细文本：头部统计 + 逐文件行。
//
// 逐文件行只剩「出过事的那几个」——失败与符号链接跳过；逐个成功不再逐行列出。
func buildTransferOutput(total, success, failed int, lines []string) string {
	header := fmt.Sprintf("total_files=%d, success_files=%d, failed_files=%d", total, success, failed)
	if len(lines) == 0 {
		return header
	}
	return header + "\n" + strings.Join(lines, "\n")
}

// fileEntry 一次传输里的一个文件（上传用本机路径、下载用远端相对路径，字段各取所需）。
type fileEntry struct {
	Path string // 本机路径（上传侧读它）
	Rel  string // 相对路径（上传：相对上传根；下载：相对下载根，POSIX 斜杠）
	Size int64
}
