package output

import (
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"sshfleet/internal/dangercheck"
)

// PrintDangerWarning 打印危险命令警告框（多命中全列、按风险降序，spec D34）。
// M4 交付可用形态；M5 统一终端呈现时可再美化。
func PrintDangerWarning(report *dangercheck.Report, forbidden bool) {
	if report == nil || len(report.Matches) == 0 {
		return
	}

	title := "⚠️  发现危险命令 ⚠️"
	footer := "是否继续执行？这可能会带来安全风险！"
	if forbidden {
		title = "🚫  发现禁止命令 🚫"
		footer = "此命令被禁止执行，程序将立即退出！"
	}

	const inner = 56
	bar := strings.Repeat("═", inner)

	risk := report.Highest()
	color := lipgloss.Color("15")
	switch risk {
	case "forbidden", "high":
		color = lipgloss.Color("9")
	case "medium":
		color = lipgloss.Color("11")
	}
	style := lipgloss.NewStyle().Foreground(color)

	pad := func(text string) string {
		w := lipgloss.Width(text)
		if w >= inner {
			return text
		}
		return text + strings.Repeat(" ", inner-w)
	}
	center := func(text string) string {
		w := lipgloss.Width(text)
		if w >= inner {
			return text
		}
		remain := inner - w
		left := remain / 2
		return strings.Repeat(" ", left) + text + strings.Repeat(" ", remain-left)
	}

	lines := []string{
		style.Render("╔" + bar + "╗"),
		style.Render("║" + center(title) + "║"),
		style.Render("╠" + bar + "╣"),
	}
	for _, m := range report.Matches {
		source := "命令"
		if report.IsScript {
			source = "脚本: " + report.ScriptPath
		}
		content := []rune(m.Content)
		if len(content) > 46 {
			content = content[:46]
		}
		lines = append(lines,
			style.Render("║"+pad("    来源: "+source)+"║"),
			style.Render("║"+pad(fmt.Sprintf("    行号: %d", m.Line))+"║"),
			style.Render("║"+pad("    内容: "+string(content))+"║"),
			style.Render("║"+pad("    分类: "+m.RuleName)+"║"),
			style.Render("║"+pad("    级别: "+strings.ToUpper(m.RiskLevel))+"║"),
			style.Render("╠"+bar+"╢"),
		)
	}
	lines = append(lines,
		style.Render("║"+center(footer)+"║"),
		style.Render("╚"+bar+"╝"),
	)
	fmt.Fprintln(os.Stdout, "\n"+strings.Join(lines, "\n"))
}
