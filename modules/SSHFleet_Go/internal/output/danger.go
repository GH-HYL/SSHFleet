package output

import (
	"fmt"
	"os"
	"strings"

	"sshfleet/internal/dangercheck"
)

// 警告框尺寸（对位旧 dangerous.py：框内宽度 = 顶部 ═ 的数量）。
const (
	dangerInnerWidth = 56
	// dangerFieldLimit 字段值（来源/内容/分类）的显示宽度上限。
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
	// 框线必歪（用户 2026-09-15 反馈「两个感叹号没对齐」）。
	title := "⚠  发现危险命令  ⚠"
	footer := "是否继续执行？这可能会带来安全风险！"
	if forbidden {
		title = "🚫  发现禁止命令  🚫"
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
		lines = append(lines,
			line("║"+pad("    来源: "+trimToWidth(source, dangerFieldLimit))+"║"),
			line("║"+pad(fmt.Sprintf("    行号: %d", m.Line))+"║"),
			line("║"+pad("    内容: "+trimToWidth(m.Content, dangerFieldLimit))+"║"),
			line("║"+pad("    分类: "+trimToWidth(m.RuleName, dangerFieldLimit))+"║"),
			line("║"+pad("    级别: "+strings.ToUpper(m.RiskLevel))+"║"),
			line("╠"+bar+"╢"),
		)
	}
	lines = append(lines,
		line("║"+center(footer)+"║"),
		line("╚"+bar+"╝"),
	)
	return strings.Join(lines, "\n")
}

// PrintDangerWarning 打印危险命令警告框（前置一个空行，与旧版一致）。
func PrintDangerWarning(report *dangercheck.Report, forbidden bool) {
	box := dangerBox(report, forbidden)
	if box == "" {
		return
	}
	fmt.Fprintln(os.Stdout, "\n"+box)
}
