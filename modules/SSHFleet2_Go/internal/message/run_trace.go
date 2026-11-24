package message

// 塔尖自己那五句「进度留痕」：只进工具日志，不上屏。
//
// **为什么是这几条、不是老版那十三条**：老版每个站成功一步都写一行（解析成功、合规通过、
// 危险检查通过、读取节点、计算统计、格式化报告、保存资源、两份 xlsx……）。新版把日志句柄
// 收到塔尖一处，各站不再自报成功——要复刻那九条，只能把句柄散回五个站（拆 ADR-0032 的分工），
// 或者让塔尖凭「没报错」替站宣布「这一步成功了」。后者更糟：日志写给没有事实的地方。
//
// 这里留下的是**塔尖自己做的动作**：它建了归档目录、它把执行期日志开了又关、它要退出。
// 事实在它手里，句子由 `message` 造，落盘由它自己调 `log`——三件事都不出界。
//
// 文案逐字照老版 `SSHFleet_Go/main.go`（`:435`／`:443`／`:484`／`:510`），一个字不改：
// 维护者照着老习惯在同一位置找同一句话。唯一没有老版对应的是最后那句：老版那条路把
// 收尾写失败吞掉，日志里少了这一截也没人知道（见 `TraceExecLogSummaryLost`）。

// TraceArchiveCreated 是归档目录建成那一句。老版用 ASCII 冒号（`成功: %s`），不做中文化。
func TraceArchiveCreated(dir string) Line {
	return Line{s: "生成归档目录成功: " + dir}
}

// TraceExecLogRotated 是执行期日志开始落盘那一句。dir 是归档目录，path 是那份日志的路径
// （相对路径，形如 `history/<目录>/SSHFleetExec.log`）。
//
// 时序说明：老版先把日志开出来再报这一句；新版日志句柄归执行站，塔尖只能在交棒前报。
// 措辞沿用老版，是因为使用者/维护者看到的是同一件事（这一次的明细落在哪），不因一句话的
// 先后而变成另一件事。
func TraceExecLogRotated(path string) Line {
	return Line{s: "执行期日志已轮转至：" + path + "（执行结束自动切回）"}
}

// TraceExecLogClosed 是执行期日志写完、切回工具日志那一句。老版排在「执行结束：成功 N 台」
// 之前，这里照同一位置。
func TraceExecLogClosed(path string) Line {
	return Line{s: "执行期日志已写完，切回工具日志：" + path}
}

// TraceExit 是收尾那一句：跑完了、日志在哪。老版只在真正跑完执行链之后写它，
// 帮助、拒绝、起不来那几条路都不写——与「已退出」这四个字的意思一致。
func TraceExit(toolLogPath string) Line {
	return Line{s: "SSHFleet已退出，日志文件：" + toolLogPath}
}

// TraceExecLogSummaryLost 是收尾那一行没能写进执行期日志时的留痕。
//
// 那一刻执行已经结束：开不出来停不下来了，停机只会在跑完之后多报一次「起不来」。但吞掉
// 也不行——归档里那份日志会缺掉这次运行的统计，事后对账才发现断了一截。所以照实记一句在
// 工具日志里：路径点名是哪一份，系统原话照抄，维护者才知道去核对哪一处。
func TraceExecLogSummaryLost(path, origin string) Line {
	return Line{s: "执行期日志收尾那一行没写进去：" + path + "（" + origin + "）——归档里这份日志缺了这次运行的统计"}
}
