package execute

// 报文（message）：工具自己陈述事实的一行文本，格式 `sshfleet.<环境>.<环节>: <描述>`。
//
// 这是报文格式在新工程的唯一定义处。老版住在 `internal/ssh/message.go`，迁移地图把那一块
// 摊给执行站——报文由执行侧产出，而执行侧不许反过来依赖判定包，所以要放到双方都够不着的
// 低处：执行站自己。
//
// 写法纪律（ADR-0021 决定第 8 条）：环节名只陈述「哪一步、什么现象」，不用 `partial`／
// `failed` 这类替使用者下结论的词——叫不叫「拒绝网络连接」是判据表里那条分类的事。
//
// 不做译文层：报文原文就是使用者看到的文本，终端、终端输出文件、`report.txt` 一律原样
// 呈现，前缀不摘、术语不翻（词表「报文」）。
const messagePrefix = "sshfleet."

// 环境取值七种，**一次搬齐**（ADR-0021 决定第 1 条、词表「报文」）。
//
// 为什么七种都在这里而不是用到哪种搬哪种：这七个取值是**格式本身的一部分**，判定与配置
// 都按这几个字符串认人；拆着搬的话，谁写错一个环境名就静默拼出一个看着合法的报文。
// 环节名不在这里——它随各摊搬（本单只搬建连那一摊）。
const (
	envConnect  = "connect"  // 建连与认证
	envSession  = "session"  // 会话与终端
	envRun      = "run"      // 命令执行
	envTransfer = "transfer" // 传输
	envPasswd   = "passwd"   // 批量改密
	envAnswer   = "answer"   // 代填
	envAbort    = "abort"    // 中断与取消
)

// msg 拼一行报文：`sshfleet.<env>.<stage>: <detail>`。
//
// detail 是英文陈述；跟在原始错误后面时把原文原样传进来（前缀是我们的、冒号后那段是别人的，
// ADR-0021 决定第 6 条）。环境名不在七种取值里时**照样拼出来**：前缀照留、环境段带一个
// 非法字符，让坏形状在产出的那一刻露出来，而不是静默混进匹配文本。
//
// 不 panic：产出报文这一刻多半已经在处理一个失败，工具不能因此再崩一次。
func msg(env, stage, detail string) string {
	if knownEnv(env) {
		return messagePrefix + env + "." + stage + ": " + detail
	}
	return messagePrefix + env + "!" + "." + stage + ": " + detail
}

// knownEnv 判环境名在不在七种取值里。
//
// 写成 switch 而不是一张包级 map：包级 map 是可变的，谁都能往里写一个值。
func knownEnv(env string) bool {
	switch env {
	case envConnect, envSession, envRun, envTransfer, envPasswd, envAnswer, envAbort:
		return true
	}
	return false
}

// abortMessage 是外部中断（Ctrl+C / 上级取消）那句，按「中断在哪一步」分两档。
//
// 还没连上的机器压根没被执行到，跟「跑到一半被打断」混为一谈，会让人以为它跑过——
// 两档的差别正是排查时先要分清的那件事。五条通道共用这一处（建连、命令与脚本、
// 代填、改密、传输入口各自的调用点），所以它比别的报文更该只有一份。
func abortMessage(connected bool) string {
	if connected {
		return msg(envAbort, abortCanceled, "interrupted after session established")
	}
	return msg(envAbort, abortConnectCanceled, "interrupted while connecting")
}
