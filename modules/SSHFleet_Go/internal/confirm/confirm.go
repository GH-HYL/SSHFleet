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

	"sshfleet/internal/cli"
	"sshfleet/internal/common"
	"sshfleet/internal/config"
	"sshfleet/internal/log"
	"sshfleet/internal/nodelist"
)

// ANSI 配色（对位旧 constants.py：横幅青 / 字段名亮青 / 值亮橙 / 确认亮黄 / 取消与提示黄）。
const (
	colorReset        = "\x1b[0m"
	colorBold         = "\x1b[1m"
	colorCyan         = "\x1b[36m"
	colorYellow       = "\x1b[33m"
	colorBlue         = "\x1b[34m"
	colorBrightYellow = "\x1b[93m"
	colorBrightCyan   = "\x1b[96m"
	colorBrightOrange = "\x1b[38;5;214m"
)

// Confirm 主干第 7 步入口。
func Confirm(args *cli.Args, nodes *nodelist.Nodes, cfg *config.Config, logger *log.Logger, in *common.Interactor) error {
	// 未输入并发数，默认使用节点数量进行并发
	if args.Number == 0 {
		args.Number = nodes.Len()
	}

	// 非交互模式：不出参数屏、不提问，但上传并发建议照旧生效（Confirm 直接返回已确认）
	if in.Disinteractive {
		if _, err := suggestUploadConcurrency(args, cfg, in, nodes.Len()); err != nil {
			return err
		}
		fmt.Printf("%s [非交互模式] 跳过执行参数确认环节，直接执行%s\n\n", colorYellow, colorReset)
		return nil
	}

	// 并发数的括号注：第一次屏说清这个数从哪来（不指定 -n = 全部并行），
	// 第二次屏（建议被采纳后的重显）说清它为什么变了。
	note := ""
	if !args.NumberGiven() {
		note = "（全部并行）"
	}
	printParamScreen(args, nodes, "", note)

	if args.Upload != "" {
		if err := showUploadContent(args.Upload); err != nil {
			return err
		}
	}

	// 上传并发建议排在参数屏之后：采纳了就把参数屏重显一遍、并发数那行凸显，
	// 免得"参数屏说 3、实际跑 10"；没采纳说明值没变，不必重显。
	// 重显的这一遍标"（已按建议修改）"，且不再标"（全部并行）"——这个数是刚确认过的，
	// 不是"不指定"的默认。
	if args.Upload != "" {
		changed, err := suggestUploadConcurrency(args, cfg, in, nodes.Len())
		if err != nil {
			return err
		}
		if changed {
			printParamScreen(args, nodes, "并发数", "（已按建议修改）")
		}
	}

	fmt.Println("\n" + strings.Repeat("═", 60))
	confirmed, err := in.Confirm("\n"+colorBrightYellow+"以上确认无误，开始执行？"+colorReset, true)
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

// printParamScreen 打印参数屏（横幅 + 信息表）。
// highlight 非空时把该标签那一行凸显；note 是并发数后面的括号注（"（全部并行）" / "（已按建议修改）"）。
func printParamScreen(args *cli.Args, nodes *nodelist.Nodes, highlight, note string) {
	title := "           SSHFleet - 执行参数确认           "
	border := strings.Repeat("═", len([]rune(title))+10)

	fmt.Printf("\n%s╔%s╗%s\n", colorCyan, border, colorReset)
	fmt.Printf("%s║  %s  ║%s\n", colorCyan, title, colorReset)
	fmt.Printf("%s╚%s╝%s\n\n", colorCyan, border, colorReset)

	printInfoTable(buildInfoTable(args, nodes, note), highlight)
}

// buildInfoTable 构建显示信息的表格数据（行序与旧版一致）。
// note 非空时跟在并发数后面，让用户知道这个数从哪来（不指定 -n 的默认，或被建议改过）。
func buildInfoTable(args *cli.Args, nodes *nodelist.Nodes, note string) [][2]string {
	identity := "登录用户"
	if args.Sudo {
		identity = "root"
	}
	t := [][2]string{{"执行身份", identity}}
	// 解释器行：只在命令 / 脚本模式出现，且只在配置的解释器不是 bash（归一判断）时才占一行
	// ——默认值不必提醒（2026-10-07 定）。--no-shell 时解释器配置不参与执行，
	// 这一行改放提示（2026-10-08 作者定：提个醒就放行，不该硬拦）。
	if (args.ModeName() == cli.ModeCommand || args.ModeName() == cli.ModeScript) &&
		args.Interpreter != "" && !cli.IsBashInterpreter(args.Interpreter) {
		value := args.Interpreter
		if args.NoShell {
			value = "原样下发，本次不生效"
		}
		t = append(t, [2]string{"解释器", value})
	}
	// 「执行模式」这一行的值取自模式名字表；其下的命令 / 路径 / 新密码是各模式自己的内容，
	// 仍按模式分派——那是结构性差异，不是叫法。
	modeLabel := [2]string{"执行模式", args.ModeName().Info().ScreenLabel}
	switch args.ModeName() {
	case cli.ModeCommand:
		t = append(t, modeLabel, [2]string{"执行命令", args.Command}, [2]string{"", ""})
	case cli.ModeScript:
		t = append(t, modeLabel, [2]string{"脚本路径", args.Script}, [2]string{"", ""})
	case cli.ModeUpload:
		t = append(t, modeLabel, [2]string{"本地路径", args.Upload}, [2]string{"远程路径", args.Path}, [2]string{"", ""})
	case cli.ModeDownload:
		t = append(t, modeLabel, [2]string{"远程路径", args.Download}, [2]string{"本地路径", args.Path}, [2]string{"", ""})
	case cli.ModePasswd:
		// 新密码是明确知道身份的凭据：按下确认之前看得见形态，但只看得到脱敏形态
		t = append(t, modeLabel, [2]string{"新密码", common.MaskSecret(args.ChangePassword)}, [2]string{"", ""})
	}
	t = append(t, answerRows(args)...)
	t = append(t,
		[2]string{"节点清单", common.MaskInlineListIf(args.CsvFile)},
		[2]string{"节点数量", fmt.Sprintf("%d", nodes.Len())},
		[2]string{"并发数", concurrentText(args, note)},
		[2]string{"", ""},
	)
	if args.ConnectTimeout != 0 {
		t = append(t, [2]string{"连接超时", fmt.Sprintf("%ds", args.ConnectTimeout)})
	}
	if args.Timeout != 0 {
		// 措辞取自名字表（执行超时 / 传输超时 / 改密超时）。改密虽走 timeout_execute 的默认值，
		// 但用户看的是「这次改密等多久」，名字表里就写着「改密超时」。
		// 名字表对「没选定模式」返零值——那就不加这一行，别摆个空标签（参数合规已保证模式必选）。
		if label := args.ModeName().Info().TimeoutLabel; label != "" {
			t = append(t, [2]string{label, fmt.Sprintf("%ds", args.Timeout)})
		}
	}
	if args.Remark != "" {
		t = append(t, [2]string{"备注", strings.TrimSpace(args.Remark)})
	}
	return t
}

// answerRows 参数屏里的代填行：每条一行，含代填内容与它的全部触发词（顺序同命令行）。
// 代填会直接改远端行为，按下确认之前得看得见这次配了什么。
func answerRows(args *cli.Args) [][2]string {
	rows := make([][2]string, 0, len(args.Answers))
	for i, entry := range args.Answers {
		rows = append(rows, [2]string{fmt.Sprintf("代填 %d", i+1), entry.Describe()})
	}
	return rows
}

// concurrentText 并发数的显示文案：note 是跟在数字后面的括号注，为空就只显示数字。
func concurrentText(args *cli.Args, note string) string {
	return fmt.Sprintf("%d%s", args.Number, note)
}

// printInfoTable 打印信息表格，对齐用 common.DisplayWidth（全角标点按 2 列计，不错位）。
// highlight 非空时，标签等于它的那一行加粗——用于"并发数被建议改了"的重显。
// 颜色不换：值一律亮橙，换色反而让那一行比别处暗（2026-10-07 作者定）。
func printInfoTable(table [][2]string, highlight string) {
	maxLabelWidth := 0
	for _, r := range table {
		if w := common.DisplayWidth(r[0]); w > maxLabelWidth {
			maxLabelWidth = w
		}
	}
	for _, r := range table {
		if r[0] == "" && r[1] == "" {
			fmt.Println()
			continue
		}
		bold := ""
		if highlight != "" && r[0] == highlight {
			bold = colorBold
		}
		label := r[0] + strings.Repeat(" ", maxLabelWidth-common.DisplayWidth(r[0]))
		fmt.Printf("%s▶ %s-→%s   %s%s%s%s\n", colorBrightCyan, label, colorReset, bold, colorBrightOrange, r[1], colorReset)
	}
}

// suggestUploadConcurrency 按配置阈值给出上传并发建议，返回是否采纳（采纳=并发数被改）。
//
// 建议规则（配置 [upload]）：
//   - 上传总大小 < small_file: 全并发（0 = 不限制，无需建议）
//   - > large_file: 串行（并发=1）
//   - 两者之间: medium_parallel
//
// 建议值先按节点数封顶：只有 1 台时"同时传 10 个"无从谈起。封顶后与当前值相同
// 就直接闭嘴——同一个数没什么可建议的（2026-09-24 作者定）。
//
// 这一行前面留一个空行：它紧跟在上传内容树之后，不留空会跟那棵树读成一块（2026-10-07 作者定）。
func suggestUploadConcurrency(args *cli.Args, cfg *config.Config, in *common.Interactor, nodeCount int) (bool, error) {
	if args.Upload == "" || cfg == nil {
		return false, nil
	}
	size := calculateUploadSize(args.Upload)
	allowed := checkConcurrencyThreshold(size, cfg)
	if allowed == 0 {
		return false, nil
	}
	if allowed > nodeCount {
		allowed = nodeCount
	}
	if allowed == args.Number {
		return false, nil
	}
	fmt.Printf("\n%s上传总大小 %s，按配置里的阈值建议同时传 %d 个（当前是 %d 个）。%s\n",
		colorYellow, common.FormatBytes(size), allowed, args.Number, colorReset)
	yes, err := in.Confirm(fmt.Sprintf("改成 %d 个？", allowed), true)
	if err != nil {
		// EOF/取消：返回 ErrCancelled 交由 main 统一退出（不在子模块内自行 os.Exit）
		return false, err
	}
	if !yes {
		return false, nil // 保留原值继续执行
	}
	args.Number = allowed
	return true, nil
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
	t := cfg.Upload
	switch {
	case size < int64(t.SmallFile):
		return 0
	case size > int64(t.LargeFile):
		return 1
	default:
		return t.MediumParallel
	}
}
