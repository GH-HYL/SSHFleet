// 终端呈现（对位旧 terminal.py）：结果明细行、统计块、退出码提示、尺寸/速度工具。
package output

import (
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"time"

	"sshfleet/internal/common"
	"sshfleet/internal/result"
	"sshfleet/internal/ssh"
)

// 常见退出码含义（Unix 通用语义，对位旧 EXIT_CODE_HINTS）。
var exitCodeHints = map[int]string{
	1:   "一般性错误",
	2:   "命令用法错误",
	126: "命令不可执行(权限不足)",
	127: "命令未找到",
	130: "被中断(SIGINT/Ctrl+C)",
	137: "被强制杀死(SIGKILL)",
	143: "被终止(SIGTERM)",
	255: "命令执行失败",
}

// ActionName 模式 → 动作中文名（状态行用）。
func ActionName(mode string) string {
	switch mode {
	case "upload":
		return "上传"
	case "download":
		return "下载"
	default:
		return "执行"
	}
}

// FormatConnStatus 连接状态行：「连接: 成功 - X.XXXs」。
func FormatConnStatus(connectSuccess bool, cost float64) string {
	status := "成功"
	if !connectSuccess {
		status = "失败"
	}
	return fmt.Sprintf("连接: %s - %.3fs", status, cost)
}

// FormatSpeed 速度显示（对位旧 format_speed）。
func FormatSpeed(bytesPerSec float64) string {
	switch {
	case bytesPerSec >= 1024*1024:
		return fmt.Sprintf("%.1fMB/s", bytesPerSec/1024/1024)
	case bytesPerSec >= 1024:
		return fmt.Sprintf("%.1fKB/s", bytesPerSec/1024)
	default:
		return fmt.Sprintf("%.0fB/s", bytesPerSec)
	}
}

// FormatBytes 字节数显示（对位旧 rich DownloadColumn 的「已传/总量」形态）。
func FormatBytes(n int64) string {
	return formatSize(n)
}

// ResultLine 单条结果的明细文本（对位旧 format_result_line）。
// 字段顺序（用户 2026-09-15 裁定）：连接 → 执行/错误 → 分类 → output 内容 → 分隔线。
// 分类提到执行下面（一眼看出结果定性），output 原文放最下面（长文本不夹在状态行中间）。
func ResultLine(r ssh.Result, mode string, category string) string {
	var lines []string
	lines = append(lines, fmt.Sprintf("【%s】 %s", r.IP, FormatConnStatus(r.ConnectSuccess, r.ConnectCostTime)))

	if r.ConnectSuccess {
		status := "失败"
		if r.ExitCode != nil && *r.ExitCode == 0 {
			status = "成功"
		}
		lines = append(lines, fmt.Sprintf("【%s】 %s: %s - %.3fs", r.IP, ActionName(mode), status, r.ExecCostTime))
	} else {
		errMsg := "未知错误"
		if r.Error != nil && *r.Error != "" {
			errMsg = *r.Error
		}
		lines = append(lines, fmt.Sprintf("【%s】 错误: %s", r.IP, errMsg))
	}

	lines = append(lines, fmt.Sprintf("【%s】 分类: %s", r.IP, category))

	if r.ConnectSuccess {
		if out := strings.TrimSpace(r.Output); out != "" {
			lines = append(lines, out)
		}
	}

	lines = append(lines, strings.Repeat("=", 50))
	return strings.Join(lines, "\n")
}

// PrintResult 输出单条结果：写 output.txt（各模式），命令模式同时打印到终端。
func PrintResult(out io.Writer, resultWriter io.Writer, r ssh.Result, mode, category string) error {
	formatted := ResultLine(r, mode, category)
	if resultWriter != nil {
		if _, err := fmt.Fprintln(resultWriter, formatted); err != nil {
			return err
		}
	}
	if mode == "execute" {
		_, _ = fmt.Fprintln(out, formatted)
	}
	return nil
}

// PrintStatistics 打印统计块（对位旧 format_statistic_results_to_terminal）。
// 配色对位旧 terminal.py：标签青 / 校验红 / 成功绿 / 失败红 / 分类黄 / 失败分类统计红 / 提示黄。
func PrintStatistics(out io.Writer, stats *result.Stats, kw *result.Keywords) {
	bar := strings.Repeat("═", 60)
	fmt.Fprintln(out, bar)
	fmt.Fprintf(out, "  总耗时：%.2f 秒\n", stats.GlobalCostTime)

	if stats.Verify == "通过" {
		fmt.Fprintf(out, "  %s节点总数：%s %d  %s完成总数：%s%d\n",
			ansiCyan, ansiReset, stats.NodesTotal, ansiCyan, ansiReset, stats.ResultsTotal)
	} else {
		fmt.Fprintf(out, "  %s节点总数：%s %d  %s完成总数：%s%d  %s总数校验：%s%s%s%s\n",
			ansiCyan, ansiReset, stats.NodesTotal, ansiCyan, ansiReset, stats.ResultsTotal,
			ansiCyan, ansiReset, ansiRed, stats.Verify, ansiReset)
	}

	if stats.FailCounts > 0 {
		fmt.Fprintf(out, "  %s成功：%s %d   %s失败：%s %d\n",
			ansiGreen, ansiReset, stats.SuccessCounts, ansiRed, ansiReset, stats.FailCounts)
	} else {
		fmt.Fprintf(out, "  %s成功：%s %d\n", ansiGreen, ansiReset, stats.SuccessCounts)
	}

	if len(stats.SortedFailCategories) > 0 {
		var known, fallback []string
		for _, c := range stats.SortedFailCategories {
			item := fmt.Sprintf("%s%s：%s%d", ansiYellow, c.Category, ansiReset, c.Count)
			if result.IsFallbackCategory(c.Category, kw) {
				fallback = append(fallback, item)
			} else {
				known = append(known, item)
			}
		}
		if len(known) > 0 {
			fmt.Fprintf(out, "  %s失败分类统计%s >>>  %s\n", ansiRed, ansiReset, strings.Join(known, "  "))
		} else {
			fmt.Fprintf(out, "  %s失败分类统计%s >>>\n", ansiRed, ansiReset)
		}
		for _, item := range fallback {
			fmt.Fprintf(out, "    %s\n", item)
		}
		if hints := exitCodeHintLine(stats.SortedFailCategories); hints != "" {
			fmt.Fprintf(out, "  %s%s%s\n", ansiYellow, hints, ansiReset)
		}
	}
	fmt.Fprintln(out, bar)
}

var exitCodeRe = regexp.MustCompile(`退出码(\d+)`)

// exitCodeHintLine 从失败分类中提取本次出现的退出码，翻译为常见退出码提示（按台数降序）。
func exitCodeHintLine(categories []result.CategoryCount) string {
	seen := map[int]int{}
	var order []int
	for _, c := range categories {
		m := exitCodeRe.FindStringSubmatch(c.Category)
		if m == nil {
			continue
		}
		code, err := strconv.Atoi(m[1])
		if err != nil {
			continue
		}
		if _, ok := exitCodeHints[code]; !ok {
			continue
		}
		if _, ok := seen[code]; !ok {
			order = append(order, code)
		}
		seen[code] = c.Count
	}
	if len(seen) == 0 {
		return ""
	}
	// 按出现台数降序（稳定：同数保持首次出现顺序）
	for i := 1; i < len(order); i++ {
		for j := i; j > 0 && seen[order[j-1]] < seen[order[j]]; j-- {
			order[j-1], order[j] = order[j], order[j-1]
		}
	}
	parts := make([]string, 0, len(order))
	for _, code := range order {
		parts = append(parts, fmt.Sprintf("%d >> %s", code, exitCodeHints[code]))
	}
	return "常见退出码: " + strings.Join(parts, "  ")
}

// elapsedText 耗时显示（对位旧 TimeElapsedColumn 的 H:MM:SS）。
func elapsedText(d time.Duration) string {
	total := int(d.Seconds())
	return fmt.Sprintf("%d:%02d:%02d", total/3600, (total%3600)/60, total%60)
}

// DisplayWidth 字符串的终端显示宽度（实现见 internal/common，全工具单一实现）。
// 框线对齐必须按它算：按字符数或按字节数算都会在中文/全角内容处错位。
func DisplayWidth(s string) int { return common.DisplayWidth(s) }

// trimToWidth 按显示宽度截断到 limit 列。
func trimToWidth(s string, limit int) string { return common.TrimToWidth(s, limit) }

// formatSize 文件大小人性化（对位旧 text_utils.format_size）。
func formatSize(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGTPE"[exp])
}
