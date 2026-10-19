package output

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"sshfleet/internal/dangercheck"
)

// 警告框尺寸（对位旧 dangerous.py：框内宽度 = 顶部 ═ 的数量）。
const (
	dangerInnerWidth = 56
	// dangerFieldLimit 字段值（来源/内容/规则）的显示宽度上限。
	// 按**显示宽度**而非字符数截断——旧版按字符数截断，中文会占 2 列，
	// 46 个中文字符即 92 列，直接把框线撑歪（用户 2026-09-15 反馈）。
	dangerFieldLimit = 46
)

// dangerBox 渲染危险命令警告框（多命中全列、按风险降序，spec D34）。
// 无命中返回空串。抽成纯函数便于断言每行等宽。
func dangerBox(report *dangercheck.Report, forbidden bool) string {
	if report == nil || len(report.Matches) == 0 {
		return ""
	}

	// 标题 / 页脚：两侧各留两个空格（旧版右侧只有 1 个，看着像少了个空格）。
	//
	// 装饰符刻意用**不带变体选择符**的 U+26A0（⚠）而非 emoji 版（⚠️）：带上
	// U+FE0F 之后，终端有的按文本呈现画 1 列、有的按 emoji 画 2 列，宽度不可预期，
	// 框线必歪（用户 2026-09-15 反馈「两个感叹号没对齐」）。禁止级同样要宽度确定：
	// 用 ✕（U+2715，Emoji 属性为 No、显示宽度 1），不用 🚫 那个宽度不稳的 emoji。
	title := "⚠  发现危险命令  ⚠"
	footer := "这条命令有风险，请确认你确实要执行它"
	if forbidden {
		title = "✕  发现禁止命令  ✕"
		footer = "此命令被禁止执行，程序将立即退出！"
	}

	const inner = dangerInnerWidth
	bar := strings.Repeat("═", inner)

	// 对位旧 constants.py：红 31 / 黄 33 / 白 37
	color := ansiWhite
	switch report.Highest() {
	case "forbidden", "high":
		color = ansiRed
	case "medium":
		color = ansiYellow
	}

	pad := func(text string) string {
		if w := DisplayWidth(text); w < inner {
			return text + strings.Repeat(" ", inner-w)
		}
		return text
	}
	center := func(text string) string {
		remain := inner - DisplayWidth(text)
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
		// 内容只是「位置提示」，超宽截断——使用者按行号回原脚本看原文即可；
		// 行号必显示、内容可截断（spec 定案 L）。示例与说明在加载时已限 30 列，不会撑框。
		lines = append(lines,
			line("║"+pad("    来源: "+trimToWidth(source, dangerFieldLimit))+"║"),
			line("║"+pad("    行号: "+formatLines(m.Lines))+"║"),
			line("║"+pad("    内容: "+trimToWidth(m.Content, dangerFieldLimit-2))+"║"),
			line("║"+pad("    规则: "+trimToWidth(m.RuleName, dangerFieldLimit))+"║"),
			line("║"+pad("    示例: "+trimToWidth(m.Example, dangerFieldLimit))+"║"),
			line("║"+pad("    说明: "+trimToWidth(m.Reason, dangerFieldLimit))+"║"),
			line("║"+pad("    级别: "+riskLevelText(m.RiskLevel))+"║"),
			// 右端用 ╣（双竖 + 左横）而不是 ╢（单竖 + 左双横）：框的左右边框都是
			// 双线，分隔行两端也得是双线，否则右边界在每一块结尾处断成单线，看着像框破了。
			line("╠"+bar+"╣"),
		)
	}
	lines = append(lines,
		line("║"+center(footer)+"║"),
		line("╚"+bar+"╝"),
	)
	return strings.Join(lines, "\n")
}

// formatLines 行号列：单行写数字，多行按 `3, 7, 12` 列出；超宽时只列出放得下的。
func formatLines(lines []int) string {
	const limit = dangerFieldLimit - 6 // 「    行号: 」占 10 列
	if len(lines) == 0 {
		return "-"
	}
	out := ""
	for i, n := range lines {
		part := strconv.Itoa(n)
		if i > 0 {
			part = ", " + part
		}
		if DisplayWidth(out+part) > limit {
			return out + " 等"
		}
		out += part
	}
	return out
}

// PrintDangerWarning 打印危险命令警告框（前置一个空行，与旧版一致）。
func PrintDangerWarning(report *dangercheck.Report, forbidden bool) {
	box := dangerBox(report, forbidden)
	if box == "" {
		return
	}
	fmt.Fprintln(os.Stdout, "\n"+box)
}

// riskLevelText 风险级别的中文名。
// 规则文件里存的是英文枚举（forbidden / high / medium / low），屏幕上给中文——
// 级别的取值要用户一眼看懂，命名规则留英文，两处各司其职。
func riskLevelText(level string) string {
	switch level {
	case "forbidden":
		return "禁止"
	case "high":
		return "高"
	case "medium":
		return "中"
	case "low":
		return "低"
	}
	return level
}
