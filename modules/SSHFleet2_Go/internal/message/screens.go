package message

import (
	"strconv"
	"strings"

	"sshfleet2/internal/contract"
)

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
func Fatal(where, body, logPath string) Line {
	if logPath != "" {
		body += "\n日志：" + logPath
	}
	s := tagLabel(ansiRed, "ERROR") + ansiYellow + " [function:" + where + "]" + ansiReset + " " + body + "\n"
	return Line{s: s, toErr: true}
}

// CancelLine 是危险提醒档答否时那一行。老版 fmt.Println 在它前面没有空行。
func CancelLine() Line {
	return Line{s: ansiYellow + "操作已取消" + ansiReset + "\n"}
}

// InputEndLine 是交互读到输入结束时那一行（老版交互器自己印）。
func InputEndLine() Line {
	return Line{s: "\n" + ansiYellow + "输入结束，操作已取消" + ansiReset + "\n"}
}

// ConfirmPrompt 是危险提醒档的问句本身（前面留一个空行）。
//
// 方括号那半截由 AskHint 给：问句与「回车会怎样」是两件事，一个在哪儿问、一个缺省是什么，
// 分开之后确认屏那句问句也能共用同一套方括号。
func ConfirmPrompt() Line {
	return Line{s: "\n" + ansiYellow + "确定继续？" + ansiReset}
}

// AskHint 是问句后面那个红色方括号：缺省答是写 [Y/n]，缺省答否写 [y/N]。
// 使用者看方括号就知道敲回车会怎样——这不是装饰。
func AskHint(defaultYes bool) string {
	if defaultYes {
		return ansiRed + "[Y/n]" + ansiReset
	}
	return ansiRed + "[y/N]" + ansiReset
}

// PromptWith 把一句问句拼成「问句 [Y/n]: 」——空格、方括号、结尾冒号只在这里成形。
//
// 单独开这个口子，是因为 `ask` 过去自己拿问句的字去接方括号（`contract.Text{S: question.S
// + " " + AskHint(...) + ": "}`）——那是「横切包自己拼一句塞给屏幕」的活标本：它读得到字，
// 是因为当时的载体字段是导出的。Line 之后它读不到字，只能走这里。
func PromptWith(question Line, defaultYes bool) Line {
	return Line{s: question.Text() + " " + AskHint(defaultYes) + ": "}
}

// notWiredHead 是过渡形态那一类话的开头：本阶段还没接上的用法都这么起头，随阶段 08 一起消失。
const notWiredHead = "这条用法还没接上："

// NotWiredKeyTools 是过渡形态：三条密钥命令随阶段 08 接线。
// 随阶段 08 消失，不许写进任何手册或帮助。
func NotWiredKeyTools() string {
	return notWiredHead + "三条密钥命令随阶段 08 接线"
}

// LogInitFailed 排「工具日志建不起来」那一屏。此时日志还没建起来，屏上不带「日志：」行。
//
// 这是塔尖唯一一处自己触发的造屏：它只把系统给的错交进来，字由这里拼。
func LogInitFailed(err error) Line {
	return Fatal("log", "初始化工具日志失败："+err.Error()+"\n请检查程序所在目录是否可写，然后重试", "")
}

// TraceDanger 是一条留给工具日志的留痕：非交互（--yes）跳过危险命令确认时写它。
// 它是**记要给日志看的**，不是给使用者的句子——所以不加上色，也不走屏幕。
//
// 形态逐字照老版 `dangercheck.Match.Note()`：级别用英文枚举、带来源行、示例与说明不论空否都留着。
func TraceDanger(m contract.DangerMatch) Line {
	note := m.RuleName + "（" + m.Level + "，来源行 " + linesText(m.Lines) + "）：" + m.Content +
		"；示例：" + m.Example + "；说明：" + m.Reason
	return Line{s: "非交互模式放行危险命令：" + note}
}

// TraceForbidden 是禁止档退场时那一行日志留痕，逐字照老版 `main`：只记原文。
func TraceForbidden(content string) Line {
	return Line{s: "命中禁止命令，已退出：" + content}
}

// linesText 把行号排成 `3, 7, 12`（日志用；框里那行要截到 46 列，另走 formatLines）。
func linesText(lines []int) string {
	parts := make([]string, 0, len(lines))
	for _, n := range lines {
		parts = append(parts, strconv.Itoa(n))
	}
	return strings.Join(parts, ", ")
}
