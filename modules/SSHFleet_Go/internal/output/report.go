// 报告文件（对位旧 report.py）：把统计结果格式化为 report.txt 写入归档目录。
// 与旧差异（spec D37）：补齐下载模式的执行参数段（旧完全没有下载分支）。
package output

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"sshfleet/internal/cli"
	"sshfleet/internal/config"
	"sshfleet/internal/result"
)

// WriteReport 生成 <归档目录>/<paths.report>。kw 用来取分类的提示语（配置里维护）。
func WriteReport(archiveDir string, stats *result.Stats, a *cli.Args, cfg *config.Config, argv []string, kw *result.Keywords) error {
	var b strings.Builder

	b.WriteString("=============================执行结果统计报告=============================\n")
	fmt.Fprintf(&b, "执行开始时间： %s\n", stats.GlobalStartTime.Format("2006-01-02 15:04:05.000000"))
	fmt.Fprintf(&b, "执行结束时间： %s\n", stats.GlobalStopTime.Format("2006-01-02 15:04:05.000000"))
	fmt.Fprintf(&b, "执行耗时： %.2f  秒\n", stats.GlobalCostTime)
	fmt.Fprintf(&b, "\n【执行命令】 \n  %s\n", strings.Join(argv, " "))

	b.WriteString("\n【执行参数】\n")
	switch {
	case a.Command != "":
		b.WriteString("  执行模式： 命令模式\n")
		fmt.Fprintf(&b, "  执行命令： %s\n", a.Command)
	case a.Script != "":
		b.WriteString("  执行模式： 脚本模式\n")
		fmt.Fprintf(&b, "  脚本路径： %s\n", a.Script)
	case a.Upload != "":
		b.WriteString("  执行模式： 上传模式\n")
		fmt.Fprintf(&b, "  本地路径： %s\n", a.Upload)
		fmt.Fprintf(&b, "  远程路径： %s\n", a.Path)
	case a.Download != "":
		// spec D37：旧版完全没有下载模式分支
		b.WriteString("  执行模式： 下载模式\n")
		fmt.Fprintf(&b, "  远程路径： %s\n", a.Download)
		fmt.Fprintf(&b, "  本地路径： %s\n", a.Path)
	}

	fmt.Fprintf(&b, "  CSV文件路径： %s\n", a.CsvFile)
	fmt.Fprintf(&b, "  节点数量： %d\n", stats.NodesTotal)
	if a.Command != "" || a.Script != "" {
		fmt.Fprintf(&b, "  并发数值： %d\n", a.Number)
	}
	if a.ConnectTimeout != 0 {
		fmt.Fprintf(&b, "  连接超时： %ds\n", a.ConnectTimeout)
	}
	if a.Timeout != 0 {
		switch {
		case a.Command != "" || a.Script != "":
			fmt.Fprintf(&b, "  执行超时： %ds\n", a.Timeout)
		case a.Upload != "" || a.Download != "":
			fmt.Fprintf(&b, "  传输超时： %ds\n", a.Timeout)
		}
	}

	b.WriteString("\n【结果统计】\n")
	fmt.Fprintf(&b, "  总耗时： %.2f  秒\n", stats.GlobalCostTime)
	fmt.Fprintf(&b, "  节点总数: %d  完成总数：%d  总数校验：%s\n", stats.NodesTotal, stats.ResultsTotal, stats.Verify)
	fmt.Fprintf(&b, "  成功: %d    失败: %d\n", stats.SuccessCounts, stats.FailCounts)
	if len(stats.SortedFailCategories) > 0 {
		parts := make([]string, 0, len(stats.SortedFailCategories))
		for _, c := range stats.SortedFailCategories {
			parts = append(parts, fmt.Sprintf("%s：%d", c.Category, c.Count))
		}
		fmt.Fprintf(&b, "  失败分类统计 -→  %s\n", strings.Join(parts, "  "))
		if cfg.Enable.ShowCategoryTips {
			if lines := categoryTipLines(stats.SortedFailCategories, kw); len(lines) > 0 {
				b.WriteString("  提示：\n")
				for _, line := range lines {
					fmt.Fprintf(&b, "    %s\n", line)
				}
			}
		}
	}

	b.WriteString("\n【IP清单统计】\n")
	// 失败分类按 IP 数量升序排列（对位旧报告）
	catKeys := make([]string, 0, len(stats.CategoryIPMap))
	for k := range stats.CategoryIPMap {
		catKeys = append(catKeys, k)
	}
	for i := 1; i < len(catKeys); i++ {
		for j := i; j > 0 && len(stats.CategoryIPMap[catKeys[j-1]]) > len(stats.CategoryIPMap[catKeys[j]]); j-- {
			catKeys[j-1], catKeys[j] = catKeys[j], catKeys[j-1]
		}
	}
	for _, category := range catKeys {
		ips := stats.CategoryIPMap[category]
		fmt.Fprintf(&b, "\n%s（%d）：\n", category, len(ips))
		for _, ip := range ips {
			fmt.Fprintf(&b, "%s\n", ip)
		}
	}
	if len(stats.SortedSuccessIPs) > 0 {
		fmt.Fprintf(&b, "\n%s（%d）：\n", stats.SuccessCategory, stats.SuccessIPsCount)
		for _, ip := range stats.SortedSuccessIPs {
			fmt.Fprintf(&b, "%s\n", ip)
		}
	}

	path := filepath.Join(archiveDir, cfg.Paths.Report)
	return os.WriteFile(path, []byte(b.String()), 0o644)
}
