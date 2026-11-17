package ssh

import (
	"fmt"
	"strings"
)

// 各摊的环节名。判定层与配置只认这几个字符串——改名等于让那一类失败落到「未归类」，
// 所以它们集中在这里、带注释，不散在调用点手打。
//
// 命名纪律（ADR-0021 决定第 8 条第 2 款）：环节名只陈述「哪一步、什么现象」，
// **不用 partial / failed 这类替使用者下结论的词**。叫不叫「上传回滚失败」是配置里 name 的事。
const (
	// session 会话与终端
	sessionCreateFailed  = "create_failed"  // 目标机不给开会话
	sessionPtyFailed     = "pty_failed"     // 目标机拒绝分配终端
	sessionChannelFailed = "channel_failed" // 拿不到输入通道
	sessionExitMissing   = "exit_missing"   // 会话结束了、没收到退出状态（库已分类）

	// run 命令执行
	runTimeout = "timeout" // 命令跑太久，到点收场
)

// transfer 传输一摊的环节名。
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
	transferTmpFailed         = "tmp_failed"         // sudo 路径的临时目录 / 文件
	transferSudoFailed        = "sudo_failed"        // sudo mv 到最终位置失败
	transferRemoteExists      = "remote_exists"      // 目标机上已有同名文件
	transferTargetSpaceShort  = "target_space_short" // 目标目录所在分区的可用量不够
	transferTmpSpaceShort     = "tmp_space_short"    // sudo 路径的暂存目录所在分区不够
	transferRolledBack        = "rolled_back"        // 上传失败后，本次写进去的文件已清干净
	transferRollbackPartial   = "rollback_partial"   // 回滚没清干净：还剩哪些路径
	transferPartialLeft       = "partial_left"       // 写到一半的文件没删掉（上传，远端）
	transferLocalPartialLeft  = "local_partial_left" // 写到一半的文件没删掉（下载，本机）
)

// PartialLeftMessage 半成品没删掉时那句陈述（ADR-0022 决定第 3 条「回滚失败要照实说」）。
//
// 半成品**不在** writtenPaths 里——它没写成功，回滚清单不认它。所以它删不掉时
// 必须自己说一句，否则使用者看到的分类是失败原因，却不知道目标机上还留着半个文件。
func PartialLeftMessage(paths []string) string {
	if len(paths) == 0 {
		return ""
	}
	return Message(EnvTransfer, transferPartialLeft,
		"half-written file left: "+strings.Join(paths, ", "))
}

// LocalPartialLeftMessage 下载侧的同一条陈述：本机的半成品没删掉。
//
// 下载不做回滚（ADR-0022 决定第 5 条），但「失败文件自己的半成品仍会删」——
// 删不掉时同样要说，否则下次重试会撞上一个残缺文件。
func LocalPartialLeftMessage(paths []string) string {
	if len(paths) == 0 {
		return ""
	}
	return Message(EnvTransfer, transferLocalPartialLeft,
		"half-written file left: "+strings.Join(paths, ", "))
}

// RollbackMessage 上传失败回滚后那句陈述（ADR-0022 决定第 3 条）。
//
// written 是本次已经写成功的文件数，removed 是其中真的删掉的那些，left 是没删掉的路径。
// 三种情形都说话——删干净了也要说，使用者在下次撞上「远程文件已存在」时要能回头看出
// 上次到底清没清。零个文件被写过时返回空串：那时目标机本来就没留下东西，无需声明。
func RollbackMessage(written, removed int, left []string) string {
	if written == 0 {
		return ""
	}
	if len(left) == 0 {
		return Message(EnvTransfer, transferRolledBack,
			fmt.Sprintf("%d of %d written files removed", removed, written))
	}
	return Message(EnvTransfer, transferRollbackPartial,
		fmt.Sprintf("%d of %d written files removed, left: %s", removed, written, strings.Join(left, ", ")))
}

// appendAbort 把「被中断」那句追加进报错原文那一格。
func appendAbort(existing *string, connectSuccess bool) *string {
	msg := AbortMessage(true, connectSuccess)
	if msg == "" {
		return existing
	}
	out := AppendMessage(deref(existing), msg)
	return &out
}

// deref 取指针里的字符串（nil 即空串）。
func deref(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// runRejected 目标机接受了连接，但拒绝执行这条命令（命令没跑起来，没有退出码）。
const runRejected = "rejected"
