package ssh

// 各摊的环节名。判定层与配置只认这几个字符串——改名等于让那一类失败落到「未归类」，
// 所以它们集中在这里、带注释，不散在调用点手打。
//
// 命名纪律（ADR-0021 决定第 8 条第 2 款）：环节名只陈述「哪一步、什么现象」，
// **不用 partial / failed 这类替使用者下结论的词**。叫不叫「部分成功」是配置里 name 的事。
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
	transferLocalFileFailed   = "local_file_failed"  // 下载侧开远程 / 建本地 / 写本地失败
	transferTmpFailed         = "tmp_failed"         // sudo 路径的临时目录 / 文件
	transferSudoFailed        = "sudo_failed"        // sudo mv 到最终位置失败
	transferRemoteExists      = "remote_exists"      // 目标机上已有同名文件
	transferFinished          = "finished"           // 这批传完了（成几个 / 共几个）
)

// appendFinished 把传输收尾那句追加进报错原文那一格（不覆盖已有内容）。
//
// 上传与下载共用这一个函数：同一句话只能有一个出处，两边各写一份迟早会分叉。
func appendFinished(existing *string, success, failed, total int) *string {
	msg := FinishedMessage(success, failed, total)
	if msg == "" {
		return existing
	}
	out := AppendMessage(deref(existing), msg)
	return &out
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
