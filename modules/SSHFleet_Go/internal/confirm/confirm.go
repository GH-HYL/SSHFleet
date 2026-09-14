// Package confirm 承载主干第 7 步：参数信息交互确认。
//
// 非交互模式显式视为已确认（spec 实现层差异：不再借用 yorn 参数值）。
// 取消（N）→ 提示「操作已取消」+ 日志 warning + 以 1 退出（经 ErrCancelled 通道）。
package confirm

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"sshfleet/internal/cli"
	"sshfleet/internal/common"
	"sshfleet/internal/config"
	"sshfleet/internal/log"
	"sshfleet/internal/nodelist"
)

// ANSI 配色（对位旧 constants.py：横幅青 / 字段名亮青 / 值亮橙 / 确认亮黄 / 取消与提示黄）。
const (
	colorReset        = "\x1b[0m"
	colorCyan         = "\x1b[36m"
	colorYellow       = "\x1b[33m"
	colorBlue         = "\x1b[34m"
	colorBrightYellow = "\x1b[93m"
	colorBrightCyan   = "\x1b[96m"
	colorBrightOrange = "\x1b[38;5;214m"
)

// Confirm 主干第 7 步入口。
func Confirm(args *cli.Args, nodes *nodelist.Nodes, cfg *config.Config, logger *log.Logger, in *common.Interactor) error {
	// 上传并发建议（y 用建议值，n 保留原值继续执行，不退出）；取消错误向上传播
	if err := checkUploadConcurrency(args, cfg, in); err != nil {
		return err
	}

	// 未输入并发数，默认使用节点数量进行并发
	if args.Number == 0 {
		args.Number = nodes.Len()
	}

	// 非交互模式：显式跳过确认，直接执行
	if in.Disinteractive {
		fmt.Printf("%s [非交互模式] 跳过执行参数确认环节，直接执行%s\n\n", colorYellow, colorReset)
		return nil
	}

	// 构建标题横幅
	title := "           SSHFleet - 执行参数确认           "
	border := strings.Repeat("═", len([]rune(title))+10)

	fmt.Printf("\n%s╔%s╗%s\n", colorCyan, border, colorReset)
	fmt.Printf("%s║  %s  ║%s\n", colorCyan, title, colorReset)
	fmt.Printf("%s╚%s╝%s\n\n", colorCyan, border, colorReset)

	printInfoTable(buildInfoTable(args, nodes))

	if args.Upload != "" {
		if err := showUploadContent(args.Upload); err != nil {
			return err
		}
	}

	fmt.Println("\n" + strings.Repeat("═", 60))
	confirmed, err := in.Confirm("\n"+colorBrightYellow+"是否执行上述参数？"+colorReset, true)
	if err != nil {
		return err
	}
	if !confirmed {
		fmt.Println(colorYellow + "操作已取消" + colorReset)
		logger.Warn("执行已取消，SSHFleet工具已退出")
		return common.ErrCancelled
	}
	fmt.Printf("SSHFleet工具%s开始执行%s......\n", colorBlue, colorReset)
	fmt.Println(strings.Repeat("=", 50))
	return nil
}

// buildInfoTable 构建显示信息的表格数据（行序与旧版一致）。
func buildInfoTable(args *cli.Args, nodes *nodelist.Nodes) [][2]string {
	t := [][2]string{{"权限类型", args.Mode}}
	switch {
	case args.Command != "":
		t = append(t, [2]string{"执行模式", "命令模式"}, [2]string{"执行命令", args.Command}, [2]string{"", ""})
	case args.Script != "":
		t = append(t, [2]string{"执行模式", "脚本模式"}, [2]string{"脚本路径", args.Script}, [2]string{"", ""})
	case args.Upload != "":
		t = append(t, [2]string{"执行模式", "上传模式"}, [2]string{"本地路径", args.Upload}, [2]string{"远程路径", args.Path}, [2]string{"", ""})
	case args.Download != "":
		t = append(t, [2]string{"执行模式", "下载模式"}, [2]string{"远程路径", args.Download}, [2]string{"本地路径", args.Path}, [2]string{"", ""})
	}
	t = append(t,
		[2]string{"CSV文件路径", args.CsvFile},
		[2]string{"节点数量", fmt.Sprintf("%d", nodes.Len())},
		[2]string{"并发数值", fmt.Sprintf("%d", args.Number)},
		[2]string{"", ""},
	)
	if args.ConnectTimeout != 0 {
		t = append(t, [2]string{"连接超时", fmt.Sprintf("%ds", args.ConnectTimeout)})
	}
	if args.Timeout != 0 {
		switch {
		case args.Command != "" || args.Script != "":
			t = append(t, [2]string{"执行超时", fmt.Sprintf("%ds", args.Timeout)})
		case args.Upload != "", args.Download != "":
			t = append(t, [2]string{"传输超时", fmt.Sprintf("%ds", args.Timeout)})
		}
	}
	if args.Remark != "" {
		t = append(t, [2]string{"备注信息", strings.TrimSpace(args.Remark)})
	}
	return t
}

// printInfoTable 打印信息表格，对齐用 lipgloss.Width（全角标点按 2 列计，不错位）。
func printInfoTable(table [][2]string) {
	maxLabelWidth := 0
	for _, r := range table {
		if w := lipgloss.Width(r[0]); w > maxLabelWidth {
			maxLabelWidth = w
		}
	}
	for _, r := range table {
		if r[0] == "" && r[1] == "" {
			fmt.Println()
			continue
		}
		label := r[0] + strings.Repeat(" ", maxLabelWidth-lipgloss.Width(r[0]))
		fmt.Printf("%s▶ %s-→%s   %s%s%s\n", colorBrightCyan, label, colorReset, colorBrightOrange, r[1], colorReset)
	}
}

// checkUploadConcurrency 按配置阈值输出上传并发建议，用户确认后应用。
//
// 建议规则（config upload.concurrency_thresholds）：
//   - file_size < small_file: 全并发（0 = 不限制，无需建议）
//   - file_size > large_file: 串行（并发=1）
//   - 两者之间: medium_concurrency
func checkUploadConcurrency(args *cli.Args, cfg *config.Config, in *common.Interactor) error {
	if args.Upload == "" || cfg == nil {
		return nil
	}
	size := calculateUploadSize(args.Upload)
	allowed := checkConcurrencyThreshold(size, cfg)
	if allowed == 0 {
		return nil
	}
	fmt.Printf("%s上传文件总大小 %s，建议并发数为 %d%s\n", colorYellow, formatSize(size), allowed, colorReset)
	yes, err := in.Confirm(fmt.Sprintf("是否使用建议并发数 %d ？", allowed), true)
	if err != nil {
		// EOF/取消：对位旧 get_user_confirmation 直接取消退出；不在子模块内自行
		// os.Exit，返回 ErrCancelled 交由 main 统一退出（2026-09-14 审计修复）
		return err
	}
	if yes {
		args.Number = allowed
	}
	// 输入 n：保留原值（未指定 -n 时后续默认使用节点数），继续执行
	return nil
}

// showUploadContent 显示上传文件/目录内容（一层树，与旧版一致）。
// 读取失败返回错误交由 main 统一退出——不在子模块内自行 os.Exit（2026-09-14 审计修复）。
func showUploadContent(uPath string) error {
	fmt.Printf("\n%s📁 上传文件/目录内容 (-u 参数):%s\n", colorYellow, colorReset)
	info, err := os.Stat(uPath)
	if err != nil {
		return fmt.Errorf("无法解析上传路径内容：%v\n请检查 -u 指定的路径是否存在且可访问", err)
	}
	name := filepath.Base(uPath)
	if !info.IsDir() {
		fmt.Printf("└── %s (文件)\n", name)
		return nil
	}
	fmt.Printf("└── %s/\n", name)
	entries, err := os.ReadDir(uPath)
	if err != nil {
		return fmt.Errorf("无法解析上传路径内容：%v\n请检查 -u 指定的路径是否存在且可访问", err)
	}
	for i, item := range entries {
		prefix := "    ├──"
		if i == len(entries)-1 {
			prefix = "    └──"
		}
		if item.IsDir() {
			fmt.Printf("%s %s/\n", prefix, item.Name())
		} else {
			fmt.Printf("%s %s\n", prefix, item.Name())
		}
	}
	return nil
}

// calculateUploadSize 上传大小：单文件取大小，目录取总大小（排除符号链接）。
func calculateUploadSize(path string) int64 {
	info, err := os.Lstat(path)
	if err != nil {
		return 0
	}
	if !info.IsDir() {
		return info.Size()
	}
	var total int64
	_ = filepath.Walk(path, func(p string, fi os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if fi.Mode().IsRegular() {
			total += fi.Size()
		}
		return nil
	})
	return total
}

// checkConcurrencyThreshold 按文件大小返回建议并发数，0 = 不限制。
func checkConcurrencyThreshold(size int64, cfg *config.Config) int {
	t := cfg.Upload.ConcurrencyThresholds
	switch {
	case size < int64(t.SmallFile):
		return 0
	case size > int64(t.LargeFile):
		return 1
	default:
		return t.MediumConcurrency
	}
}

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
