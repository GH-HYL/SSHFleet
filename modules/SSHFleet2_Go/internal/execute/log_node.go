package execute

// 执行期日志的节点明细：一台跑完之后，写进 `SSHFleetExec.log` 的那几级。
//
// 版式照老版 `output/reporter.go` 的 `logNode`：成功一行；失败五级——连接 → 执行 →
// 错误详情 → 输出明细 → 分类，一级一条，每条都带节点前缀与时间戳。
//
// **为什么不像给使用者看的那一段字一样合成一段**（开发者 2026-11-02 的原话）：
// 「终端输出和 output 是给人看的，当然要一致，log 是记录的」。屏幕与 `terminal-output.txt`
// 一行一台才看得下去；日志是留给维护者的现场——要能按 IP 过滤、要分得清「哪一步、什么
// 现象」，所以一级一条、每行都带前缀。老版就是这么分的，这里照搬。

import (
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"sshfleet2/internal/contract"
	"sshfleet2/internal/log"
	"sshfleet2/internal/message"
)

// 执行期日志里单个节点的「输出明细」上限：超出只报数，完整内容在 terminal-output.txt。
//
// 为什么要封顶：`cat 大文件` 这类命令能产出上万行，日志不能无上限地长。
const (
	maxOutputLines   = 50
	maxOutputLineLen = 500
)

// logNode 把一台的结果按运行事件写进执行期日志。
//
// 五级顺序与老版一致：连接结果 → 执行结果 → 失败原因原文 → 输出明细 → 分类。成功那条
// 只写一行（目标机常是上千台，铺开五级会把日志刷满）。
func logNode(lg *log.Logger, res contract.NodeResult, mode contract.Mode, termPath, user string) {
	if lg == nil {
		return
	}
	ip := "【" + res.IP + "】"

	if message.IsSuccess(res, mode) {
		lg.Success(ip + "成功：" + successDetail(res, mode))
		return
	}

	// 一级：连接。连不上那台，报错原文就是唯一的原因来源，逐行写下来。
	//
	// 「用户 xxx」带上：排查时第一个要问的就是「用哪个账号登的」。**登录方式那一截没有**——
	// 老版记的是握手时实际用上的那一种，新版纸里没有这一格，凭「配了密钥没有」推会推错
	// （两个都配时由库自己挑），宁缺不猜。
	if res.Connected {
		lg.Success(fmt.Sprintf("%s连接成功%s，耗时 %s", ip, userNote(user), seconds3(res.ConnectCost)))
	} else {
		logMultiline(lg.Error, ip, "连接失败"+userNote(user)+"：", message.UnknownIfEmpty(res.ErrorText))
	}

	// 二级：执行结果。只写失败——成功在上面已经提前 return。
	if res.Connected {
		code := "无"
		if res.ExitCode != nil {
			code = strconv.Itoa(*res.ExitCode)
		}
		lg.Error(fmt.Sprintf("%s%s失败，退出码 %s，耗时 %s", ip, message.LogLabel(mode), code, seconds3(res.Cost)))
	}

	// 三级：失败原因原文。连接失败那一步已经写过，这里补「连上了却没跑成」的原因。
	if res.Connected && res.ErrorText != "" {
		logMultiline(lg.Error, ip, "错误详情：", res.ErrorText)
	}

	// 四级：输出明细。与「错误详情」逐字相同的输出不再重复写一遍。
	if res.Output != "" && !sameAsError(res) {
		logOutputBlock(lg.Warn, ip, res.Output, termPath)
	}

	// 五级：分类。
	lg.Info(ip + "分类: " + res.Category)
}

// userNote 日志里「，用户 xxx」那一截；没给用户时给空串（不占位）。
func userNote(user string) string {
	if strings.TrimSpace(user) == "" {
		return ""
	}
	return "，用户 " + user
}

// successDetail 成功那一行的后半截：「连接 0.029s，执行 0.003s」。
func successDetail(res contract.NodeResult, mode contract.Mode) string {
	return fmt.Sprintf("连接 %s，%s %s", seconds3(res.ConnectCost), message.ActionName(mode), seconds3(res.Cost))
}

// seconds3 耗时写成 `0.123s`（老版就是三位小数带单位）。
func seconds3(d time.Duration) string {
	return fmt.Sprintf("%.3fs", d.Seconds())
}

// logMultiline 把可能含多行的原文按行写入日志：首行接在 head 后面，续行缩进对齐，
// 每行都带节点前缀——既能按 IP 过滤，也保证每行时间戳齐全。
func logMultiline(fn func(...any), ip, head, text string) {
	lines := splitLines(text)
	if len(lines) == 0 {
		fn(ip + head)
		return
	}
	for i, ln := range lines {
		if i == 0 {
			fn(ip + head + truncateLine(ln))
			continue
		}
		fn(ip + "  " + truncateLine(ln))
	}
}

// logOutputBlock 把失败节点的输出明细按行写入日志（带行数抬头与上限）。
func logOutputBlock(fn func(...any), ip, output, termPath string) {
	lines := splitLines(output)
	if len(lines) == 0 {
		return
	}
	fn(fmt.Sprintf("%s输出明细（%d 行）：", ip, len(lines)))

	shown, rest := lines, 0
	if len(lines) > maxOutputLines {
		shown, rest = lines[:maxOutputLines], len(lines)-maxOutputLines
	}
	for _, ln := range shown {
		fn(ip + "  " + truncateLine(ln))
	}
	if rest > 0 {
		fn(fmt.Sprintf("%s  ……其余 %d 行省略（完整内容见 %s）", ip, rest, termPath))
	}
}

// sameAsError 输出明细与失败原因是否逐字相同（相同就不必再写一遍）。
func sameAsError(res contract.NodeResult) bool {
	if res.ErrorText == "" || res.Output == "" {
		return false
	}
	return strings.Join(splitLines(res.Output), "\n") == strings.Join(splitLines(res.ErrorText), "\n")
}

// splitLines 按行拆分并去掉空行：日志每行都已带前缀与时间戳，空行只会把版面拉稀。
func splitLines(text string) []string {
	raw := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	out := make([]string, 0, len(raw))
	for _, ln := range raw {
		ln = strings.TrimRight(ln, " \t\r")
		if strings.TrimSpace(ln) == "" {
			continue
		}
		out = append(out, ln)
	}
	return out
}

// truncateLine 单行超长时截断（超长行多为 JSON / base64 这类内容）。
func truncateLine(s string) string {
	if utf8.RuneCountInString(s) <= maxOutputLineLen {
		return s
	}
	return string([]rune(s)[:maxOutputLineLen]) + "…"
}
