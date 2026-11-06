package message

// 表格里的行：`terminal-output.xlsx` 的三列与逐行输出。
//
// 为什么另立一份而不是从 `ResultLineText` 切：终端那段是一整块给人连着读的（带 IP 前缀、
// 带分隔线），表格是三列能筛能搜的。两者的**字**同源（连接状态、执行状态、分类名都调同一
// 个函数），**排法**不同——这一层只摆行，不出新话。

import (
	"strings"

	"sshfleet2/internal/contract"
)

// ResultRow 是表格里的一行：事件（第二列）与详情（第三列）。
//
// 第一列是这一行的 IP，由写表的人补——同一台的行连着写，IP 只在外面说一次就够。
//
// Output 为真表示详情是输出原文的一行：那一格靠上对齐、不折行，其余行不设样式。
// Separator 为真表示这是结果之间那条分隔行：不写值，只铺一格底色。
type ResultRow struct {
	Event     string
	Detail    string
	Output    bool
	Separator bool
}

// 表格里那三列的固定标签（老版 output/xlsx.go 的原话）。
const (
	RowEventError  = "错误"
	RowEventOutput = "标准输出和错误输出"
)

// TerminalHeaders 是终端那张表的三列列名。
func TerminalHeaders() []string { return []string{"IP地址", "事件类型", "内容详情"} }

// ResultsHeaders 是结果明细那张表的十五列列名。列序与列名一字不改——使用者按列名找东西。
func ResultsHeaders() []string {
	return []string{
		"序号", "IP", "端口", "用户名", "连接成功", "退出码", "连接耗时",
		"执行耗时", "传输字节", "文件总数", "成功文件数", "失败文件数", "分类", "报错原文", "输出内容",
	}
}

// ResultRows 把一台结果摊成表格要的那几行。
//
// 次序照老版：连接 → 执行 → 错误（只有失败那台有）→ 输出逐行 → 分类 → 分隔行。
// 两处故意与终端不同，都不是笔误：
//   - **连接失败那台照样有一行「执行: 失败」**——老版表格无条件印，终端文本不印；
//   - **错误那一行的第二列是「错误」**，详情直接是原文（终端那边写作「错误详情: …」）。
func ResultRows(res contract.NodeResult, mode contract.Mode) []ResultRow {
	rows := []ResultRow{
		{Event: connStatus(res)},
		{Event: statusLine(res, mode)},
	}

	if !IsSuccess(res, mode) && res.ErrorText != "" {
		rows = append(rows, ResultRow{Event: RowEventError, Detail: res.ErrorText})
	}

	// 输出逐行：空行照样占一行（空行可能是输出自身的分段，`ls -l`／`df -h` 都靠它）。
	if res.Connected && res.Output != "" {
		for _, line := range strings.Split(strings.ReplaceAll(res.Output, "\r\n", "\n"), "\n") {
			rows = append(rows, ResultRow{Event: RowEventOutput, Detail: line, Output: true})
		}
	}

	rows = append(rows,
		ResultRow{Event: "分类: " + res.Category},
		ResultRow{Separator: true},
	)
	return rows
}
