package execute

// 各摊的环节名。判定层与配置只认这几个字符串——改名等于让那一类失败落到「未归类」，
// 所以集中在这里、带注释，不散在调用点手打。
//
// 命名纪律（ADR-0021 决定第 8 条第 2 款）：环节名只陈述「哪一步、什么现象」，
// 不用 partial／failed 这类替使用者下结论的词。
//
// 本单只搬命令模式用得到的这几条；传输、代填、改密的环节名随各自那一单搬。

const (
	// session 会话与终端
	sessionCreateFailed  = "create_failed"  // 目标机不给开会话
	sessionPtyFailed     = "pty_failed"     // 目标机拒绝分配终端
	sessionChannelFailed = "channel_failed" // 拿不到输入通道
	sessionExitMissing   = "exit_missing"   // 会话结束了、没收到退出状态（库已分类）

	// run 命令执行
	runTimeout  = "timeout"  // 命令跑太久，到点收场
	runRejected = "rejected" // 目标机接受了连接，但拒绝执行这条命令（没跑起来，没有退出码）
)
