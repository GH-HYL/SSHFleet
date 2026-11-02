package message

import (
	"strconv"
	"strings"

	"sshfleet2/internal/contract"
)

// 危险命令警告框：老版 output/danger.go 的同一套渲染（框宽 56、字段截 46 列、
// 装饰符不带变体选择符）。框线每行等宽是硬要求——使用者 2026-09-15 报过「两个
// 感叹号没对齐」。

const (
	dangerInnerWidth = 56
	dangerFieldLimit = 46
)

// 全书的色码只在这一处：统计块、危险框、确认屏、节点表都从这里取。
const (
	ansiReset  = "\x1b[0m"
	ansiRed    = "\x1b[31m"
	ansiGreen  = "\x1b[32m"
	ansiYellow = "\x1b[33m"
	ansiWhite  = "\x1b[37m"
	ansiDim    = "\x1b[90m" // 暗灰：统计块的「提示：」块用它，与内容同色
)

// DangerBox 排一张危险命令框。没有命中返回空串。
//
// forbidden 为真时是「发现禁止命令」（程序要立即退出），否则是「发现危险命令」（问人）。
func DangerBox(report contract.DangerReport, forbidden bool) Line {
	if len(report.Matches) == 0 {
		return Line{}
	}

	title := "⚠  发现危险命令  ⚠"
	footer := "该命令命中危险规则，请确认后继续执行"
	if forbidden {
		title = "✕  发现禁止命令  ✕"
		footer = "此命令被禁止执行，程序将立即退出！"
	}

	const inner = dangerInnerWidth
	bar := strings.Repeat("═", inner)

	color := ansiWhite
	switch highestLevel(report.Matches) {
	case contract.RiskForbidden, contract.RiskHigh:
		color = ansiRed
	case contract.RiskMedium:
		color = ansiYellow
	}

	center := func(text string) string {
		remain := inner - displayWidth(text)
		if remain <= 0 {
			return text
		}
		left := remain / 2
		return strings.Repeat(" ", left) + text + strings.Repeat(" ", remain-left)
	}
	line := func(s string) string { return color + s + ansiReset }

	lines := []string{
		line("╔" + bar + "╗"),
		line("║" + center(title) + "║"),
		line("╠" + bar + "╣"),
	}
	for _, m := range report.Matches {
		source := "命令"
		if report.IsScript {
			source = "脚本: " + report.ScriptPath
		}
		// 每块固定七行：来源 / 行号 / 内容 / 规则 / 示例 / 说明 / 级别。
		// 内容只是位置提示，超宽截断——按行号回原脚本看原文即可。
		lines = append(lines,
			line("║"+padTo("    来源: "+trimToWidth(source, dangerFieldLimit), inner)+"║"),
			line("║"+padTo("    行号: "+formatLines(m.Lines), inner)+"║"),
			line("║"+padTo("    内容: "+trimToWidth(m.Content, dangerFieldLimit-2), inner)+"║"),
			line("║"+padTo("    规则: "+trimToWidth(m.RuleName, dangerFieldLimit), inner)+"║"),
			line("║"+padTo("    示例: "+trimToWidth(m.Example, dangerFieldLimit), inner)+"║"),
			line("║"+padTo("    说明: "+trimToWidth(m.Reason, dangerFieldLimit), inner)+"║"),
			line("║"+padTo("    级别: "+riskLevelText(m.Level), inner)+"║"),
			// 右端用 ╣：框的左右边框都是双线，分隔行两端也得是双线。
			line("╠"+bar+"╣"),
		)
	}
	lines = append(lines,
		line("║"+center(footer)+"║"),
		line("╚"+bar+"╝"),
	)
	// 老版在框前补一个空行（fmt.Fprintln(os.Stdout, "\n"+box)）。
	return Line{s: "\n" + strings.Join(lines, "\n") + "\n"}
}

// highestLevel 报命中的最高风险档；无命中返回空串。命中已由闸门按档排好序。
func highestLevel(ms []contract.DangerMatch) string {
	if len(ms) == 0 {
		return ""
	}
	return ms[0].Level
}

// formatLines 行号列：单行写数字，多行按 `3, 7, 12` 列出；超宽时截在「等」。
func formatLines(lines []int) string {
	const limit = dangerFieldLimit - 6
	if len(lines) == 0 {
		return "-"
	}
	out := ""
	for i, n := range lines {
		part := strconv.Itoa(n)
		if i > 0 {
			part = ", " + part
		}
		if displayWidth(out+part) > limit {
			return out + " 等"
		}
		out += part
	}
	return out
}

// riskLevelText 风险级别的中文名：规则文件里存英文枚举，屏幕上给中文。
func riskLevelText(level string) string {
	switch level {
	case contract.RiskForbidden:
		return "禁止"
	case contract.RiskHigh:
		return "高"
	case contract.RiskMedium:
		return "中"
	case contract.RiskLow:
		return "低"
	}
	return level
}
