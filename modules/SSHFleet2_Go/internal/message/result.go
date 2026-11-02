package message

// 结果行：一台跑完之后，屏幕、`terminal-output.txt`、执行期日志三处共用的那一段字。
//
// 版式逐字照老版 `output/terminal.go` 的 `ResultLine`，字段顺序是 2026-09-15 裁定过的：
// 连接 → 执行/错误 → 分类 → 原文 → 分隔线。分类提到执行下面一眼看出定性，原文放最下面，
// 长输出不夹在状态行中间。
//
// 为什么住这里：三个去处要的是同一段字，而字只在这一处成形（ADR-0032 第四条「唯一拼句处」）。
// 三处各拼一遍，就是三个会各自漂的来源——改一处漏两处，屏幕上与归档里对不上。

import (
	"fmt"
	"strings"

	"sshfleet2/internal/contract"
)

// resultRuleWidth 结果行末尾那条分隔线的宽度：老版就是 50 个等号。
const resultRuleWidth = 50

// unknownError 连接失败、原文又是空的时候那句兜底（老版同字）。
const unknownError = "未知错误"

// NodeTag 是每一行前面的节点前缀「【IP】」。
//
// 屏幕那一行、执行期日志的每一级、报告里按类归拢的清单都用它——同一个节点在三处必须是
// 同一个写法，否则按 IP 过滤就漏。执行站也从这里取（它 import message，不自己拼）。
func NodeTag(ip string) string {
	return "【" + ip + "】"
}

// tagged 是「节点前缀 + 空格 + 这一行的正文」：结果行里那五行都走它。
func tagged(ip, rest string) string {
	return NodeTag(ip) + " " + rest
}

// ResultLine 一台的结果，末尾带换行：屏幕与 `terminal-output.txt` 拿的是它。
func ResultLine(res contract.NodeResult, mode contract.Mode) Line {
	return Line{s: ResultLineText(res, mode) + "\n"}
}

// ResultLineText 同一段字，不带结尾换行：执行期日志按自己的格式收尾，多一个换行就在日志里
// 留一条空行。
func ResultLineText(res contract.NodeResult, mode contract.Mode) string {
	ip := res.IP
	lines := make([]string, 0, 6)
	lines = append(lines, tagged(ip, connStatus(res)))

	if res.Connected {
		lines = append(lines, tagged(ip, statusLine(res, mode)))
		// 连上了却没跑成：失败详情也要在明细里看得到。这一行原先只在连接失败时才有，
		// 那类结果的细节只能事后翻表格（老版 D35 改的那一处）。
		if !IsSuccess(res, mode) && res.ErrorText != "" {
			lines = append(lines, tagged(ip, "错误详情: "+showControlChars(res.ErrorText)))
		}
	} else {
		lines = append(lines, tagged(ip, "错误: "+showControlChars(UnknownIfEmpty(res.ErrorText))))
	}

	lines = append(lines, tagged(ip, "分类: "+res.Category))

	// 只有连上了才有原文可说：连都连不上时那一格是空的，印一行空白只会让人以为「有输出但看不见」。
	if res.Connected && res.Output != "" {
		lines = append(lines, showControlChars(res.Output))
	}
	lines = append(lines, strings.Repeat("=", resultRuleWidth))
	return strings.Join(lines, "\n")
}

// connStatus 连接状态那半截：「连接: 成功 - 0.123s」。
func connStatus(res contract.NodeResult) string {
	status := "失败"
	if res.Connected {
		status = "成功"
	}
	return fmt.Sprintf("连接: %s - %.3fs", status, res.ConnectCost.Seconds())
}

// statusLine 执行状态那半截：「执行: 成功 - 0.456s」。
func statusLine(res contract.NodeResult, mode contract.Mode) string {
	status := "失败"
	if IsSuccess(res, mode) {
		status = "成功"
	}
	return fmt.Sprintf("%s: %s - %.3fs", ActionName(mode), status, res.Cost.Seconds())
}

// IsSuccess 这一台算不算成功。判据只有一句：分类是本模式的成功分类（ADR-0035 决定四）。
//
// 这不是第二套判法——分类名与成功分类名出自同一张模式名字表，这里只是把同一个名字对一次。
// 结算站那边对的是同一个名字，两边对出来的结果必然一致。结果行与执行期日志都从这里取，
// 免得两处各判一次、各漂各的。
func IsSuccess(res contract.NodeResult, mode contract.Mode) bool {
	name := SuccessCategory(mode)
	return name != "" && res.Category == name
}

// UnknownIfEmpty 原文为空时给那句兜底（老版同一句）。
func UnknownIfEmpty(text string) string {
	if strings.TrimSpace(text) == "" {
		return unknownError
	}
	return text
}

// controlCharPictures 会改写终端屏幕的控制字符 → 看得见的写法（Unicode 控制图示）。
//
// 远端输出是任意字节流：回车、退格、NUL 交给终端就被当成控制指令——行中间的回车把行首
// 覆盖成空白、孤立的那些把整行吃掉，屏幕上只剩一条分隔线，看起来像「没有输出」，而归档
// 文件里字字都在（2026-10-20 真机取证：Windows 连 iBMC 跑 `ipmcget -d health`）。
//
// 换成图示而不是删掉：删了就看不出「输出里原本有什么」，下次同样的问题又只能靠猜。
// 制表符与换行不在表里——制表符按列宽展开正是想要的排版，换行在采集侧已与 CR 归一。
var controlCharPictures = map[rune]string{
	'\r': "␍",
	'\b': "␈",
	0x00: "␀",
	'\v': "␋",
	'\f': "␌",
	0x7f: "␡",
}

// showControlChars 逐个替换上表里的控制字符，其余字符原样保留。
func showControlChars(s string) string {
	needsWork := false
	for _, r := range s {
		if _, ok := controlCharPictures[r]; ok {
			needsWork = true
			break
		}
	}
	if !needsWork {
		return s // 常见路径：绝大多数输出里一个控制字符都没有
	}
	var b strings.Builder
	b.Grow(len(s) + 8)
	for _, r := range s {
		if pic, ok := controlCharPictures[r]; ok {
			b.WriteString(pic)
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}
