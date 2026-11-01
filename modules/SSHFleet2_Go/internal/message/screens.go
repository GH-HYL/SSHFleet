package message

import "sshfleet2/internal/contract"

// 起不来与拒绝那几屏：版式照老版 main.go 的 fatal 与危险检测那几处，一个字不改。
//
// 老版形态：`\x1b[31m[ERROR]\x1b[0m\x1b[33m [function:<环节>]\x1b[0m <正文>\n`
// 正文自己按「问题 / 原因 / 提示」分行，末尾再附一行「日志：<绝对路径>」。

// 三段话的标签。值来自老版 common/errtext.go，一个字不动。
const (
	reasonLabel = "\n原因："
	hintLabel   = "\n提示："
)

// Problem 拼「问题 / 原因 / 提示」三行：说得出为什么、也有怎么改可写的报错用这个。
func Problem(problem, reason, hint string) string {
	return problem + reasonLabel + reason + hintLabel + hint
}

// ProblemReason 拼「问题 / 原因」两行：有系统原文、写不出有意义的提示时用。
func ProblemReason(problem, reason string) string {
	return problem + reasonLabel + reason
}

// ProblemHint 拼「问题 / 提示」两行：纯形态判定、没有系统原文可借时用。
func ProblemHint(problem, hint string) string {
	return problem + hintLabel + hint
}

// ProblemOrigin 拼「问题 / 出处」两行：判据文件里某一条写坏时用，出处指到那一项该怎么写。
//
// 老版这一处用的就是「出处：」这个标签（不是「原因：」也不是「提示：」），逐字照搬。
func ProblemOrigin(problem, origin string) string {
	return problem + "\n出处：" + origin
}

// Fatal 排一屏致命错误：红色 [ERROR] 头、黄色环节标签、正文，末尾附日志路径。
//
// 日志还没建起来时 logPath 传空串，那一行就不出现——老版同一判据。
func Fatal(where, body, logPath string) contract.Text {
	if logPath != "" {
		body += "\n日志：" + logPath
	}
	s := ansiRed + "[ERROR]" + ansiReset + ansiYellow + " [function:" + where + "]" + ansiReset + " " + body + "\n"
	return contract.Text{S: s, ToErr: true}
}

// RefuseLine 排一屏走标准输出的拒绝：危险框已经由 DangerBox 排好，这里只补那一句取消。
func RefuseLine(text string) contract.Text {
	return contract.Text{S: text + "\n"}
}

// CancelLine 是危险提醒档答否时那一行。老版 fmt.Println 在它前面没有空行。
func CancelLine() contract.Text {
	return contract.Text{S: ansiYellow + "操作已取消" + ansiReset + "\n"}
}

// InputEndLine 是交互读到输入结束时那一行（老版交互器自己印）。
func InputEndLine() contract.Text {
	return contract.Text{S: "\n" + ansiYellow + "输入结束，操作已取消" + ansiReset + "\n"}
}

// ConfirmPrompt 是危险提醒档的问句：前面一个换行、问句、红色 [y/N]、冒号与一个空格。
// 形态照老版交互器——缺了 [y/N] 使用者不知道能敲什么。
func ConfirmPrompt() contract.Text {
	return contract.Text{S: "\n" + ansiYellow + "确定继续？" + ansiReset + " " + ansiRed + "[y/N]" + ansiReset + ": "}
}

// NotWiredLine 是过渡形态：流水线还没接上，放行之后无处可去。
// 随阶段 04 消失，不许写进任何手册或帮助。
func NotWiredLine() contract.Text {
	return contract.Text{S: "这条用法还没接上：流水线随阶段 04 接线，本阶段只到闸门\n"}
}

// NotWiredKeyTools 是过渡形态：三条密钥命令随阶段 08 接线。
// 随阶段 08 消失，不许写进任何手册或帮助。
func NotWiredKeyTools() string {
	return "这条用法还没接上：三条密钥命令随阶段 08 接线"
}

// LogInitFailed 排「工具日志建不起来」那一屏。此时日志还没建起来，屏上不带「日志：」行。
//
// 这是塔尖唯一一处自己触发的造屏：它只把系统给的错交进来，字由这里拼。
func LogInitFailed(err error) contract.Text {
	return Fatal("log", "初始化工具日志失败："+err.Error()+"\n请检查程序所在目录是否可写，然后重试", "")
}

// TraceDanger 是一条留给工具日志的留痕：非交互（--yes）跳过危险命令确认时写它。
// 它是**记要给日志看的**，不是给使用者的句子——所以不加上色，也不走屏幕。
func TraceDanger(m contract.DangerMatch) contract.Text {
	note := m.RuleName + "（" + riskLevelText(m.Level) + "）：" + m.Content
	if m.Example != "" {
		note += "；示例：" + m.Example
	}
	if m.Reason != "" {
		note += "；说明：" + m.Reason
	}
	return contract.Text{S: "非交互模式放行危险命令：" + note}
}
