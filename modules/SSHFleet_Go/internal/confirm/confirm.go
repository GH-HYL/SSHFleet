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
	"unicode"

	"sshfleet/internal/cli"
	"sshfleet/internal/common"
	"sshfleet/internal/config"
	"sshfleet/internal/log"
	"sshfleet/internal/nodelist"
)

// ANSI 配色（对位旧 constants.py：横幅青 / 字段名亮青 / 值亮橙 / 确认亮黄 / 取消与提示黄）。
// var 而非 const：值经 common.ANSI 取，颜色能力不可用时为空串。
var (
	colorReset        = common.ANSI("\x1b[0m")
	colorBold         = common.ANSI("\x1b[1m")
	colorCyan         = common.ANSI("\x1b[36m")
	colorYellow       = common.ANSI("\x1b[33m")
	colorBlue         = common.ANSI("\x1b[34m")
	colorBrightYellow = common.ANSI("\x1b[93m")
	colorBrightCyan   = common.ANSI("\x1b[96m")
	colorBrightOrange = common.ANSI("\x1b[38;5;214m")
)

// Confirm 主干第 7 步入口。
func Confirm(args *cli.Args, nodes *nodelist.Nodes, cfg *config.Config, logger *log.Logger, in *common.Interactor) error {
	// 未输入并发数，默认使用节点数量进行并发
	if args.Number == 0 {
		args.Number = nodes.Len()
	}

	// --no-shell 会忽略 sudo：sudo 是工具替命令加的那层东西，而 --no-shell 的要求正是
	// "工具不附加任何东西"——丢掉不算错，静默丢掉才算。所以先提醒一句、让用户确认。
	// 两种模式都走：非交互（--yes）下留痕 + 上屏一行。
	if args.NoShell && args.Sudo {
		if err := warnNoShellDropsSudo(in, logger); err != nil {
			return err
		}
	}

	// [locale] english = true 时提示语统一英文，代填值（第 1 列）再写中文就是必失配的
	// 逻辑错误——提醒后让用户回命令行改掉。触发词不管：服务端报文（密码过期等）不受
	// english 影响，仍是中文，那一半照写。english = false 时不进这段检查（作者口径）。
	if err := gateChineseAnswerValues(args, in, logger); err != nil {
		return err
	}

	// 参数确认的入参：Prompt 给交互用，Trace / SkipLine 给非交互（--yes）跳过用。
	confirmReq := common.ConfirmReq{
		Prompt:     "\n" + colorBrightYellow + "以上确认无误，开始执行？" + colorReset,
		DefaultYes: true,
		Trace:      "非交互模式（--yes）跳过执行选项/参数确认，直接执行",
		SkipLine:   fmt.Sprintf("%s [非交互模式] 跳过执行选项/参数确认环节，直接执行%s\n\n", colorYellow, colorReset),
	}

	// 非交互模式：不出参数屏、不提问，但上传并发建议照旧生效（Confirm 直接返回已确认）
	if in.Disinteractive {
		if _, err := suggestUploadConcurrency(args, cfg, in, nodes.Len()); err != nil {
			return err
		}
		_, err := in.Confirm(confirmReq)
		return err
	}

	// 并发数的括号注：第一次屏说清这个数从哪来（不指定 -n = 全部并行），
	// 第二次屏（建议被采纳后的重显）说清它为什么变了。
	note := ""
	if !args.NumberGiven() {
		note = "（全部并行）"
	}
	fmt.Print(paramScreenText(args, nodes, "", note))

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
			fmt.Print(paramScreenText(args, nodes, "并发数", "（已按建议修改）"))
		}
	}

	fmt.Println("\n" + strings.Repeat("═", 60))
	confirmed, err := in.Confirm(confirmReq)
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

// gateChineseAnswerValues english = true 时代填值（第 1 列）不得为中文的拦截门。
// 交互下先打一段说明再问一句，默认 n——回车即退出，让他回命令行改代填值、或把 english
// 改成 false，不替他偷改参数。触发词（第 2 列起）不查：服务端报文（密码过期、sshd 提示）
// 不受 english 影响，仍可能是中文。
// 非交互（--yes）没有「他」可提醒，而这是一定失配的逻辑错误，放行就是整批白跑——所以
// 非交互直接取消并留痕，不给「跳过确认就继续」的口子（2026-10-28 作者口径：提醒然后退出）。
func gateChineseAnswerValues(args *cli.Args, in *common.Interactor, logger *log.Logger) error {
	// english = false：提示语跟目标机走，中文代填值是合法配置，这段检查整个不存在。
	if !args.English {
		return nil
	}
	var named []string
	for i, ans := range args.Answers {
		if hasHan(ans.Value) {
			named = append(named, fmt.Sprintf("第 %d 条代填值是「%s」", i+1, ans.Value))
		}
	}
	if len(named) == 0 {
		return nil
	}
	// 提醒走 in.Out（与 Notice 同一个出口）：提示不直接写 stdout，测试才捕得到。
	if !in.Disinteractive {
		fmt.Fprintf(in.Out, "%s⚠ [locale] english = true，但代填值里有中文%s\n", colorYellow, colorReset)
		fmt.Fprintln(in.Out, "  english = true 时目标机的提示语一律英文，代填值（第 1 列）按英文准备才对：")
		for _, line := range named {
			fmt.Fprintln(in.Out, "    "+line)
		}
		fmt.Fprintln(in.Out, "  触发词不受此限——服务端报文（如密码过期）不受 english 影响，仍可能是中文。")
		fmt.Fprintln(in.Out, "  要跑中文代填值，请把配置里 [locale] 的 english 改成 false。")
	}
	if in.Disinteractive {
		fmt.Fprintln(in.Out, colorYellow+"⚠ [locale] english = true 但代填值含中文：已取消执行（回命令行改代填值，或把 english 改成 false）"+colorReset)
		if logger != nil { // 测试可不带日志口；生产路径恒有
			logger.Warn("执行已取消（english = true 与中文代填值同时给出）")
		}
		return common.ErrCancelled
	}
	ok, err := in.Confirm(common.ConfirmReq{
		Prompt:     "\n" + colorYellow + "仍要继续执行？" + colorReset,
		DefaultYes: false,
	})
	if err != nil {
		return err
	}
	if !ok {
		fmt.Fprintln(in.Out, colorYellow+"操作已取消"+colorReset)
		if logger != nil { // 测试可不带日志口；生产路径恒有
			logger.Warn("执行已取消（english = true 与中文代填值同时给出）")
		}
		return common.ErrCancelled
	}
	return nil
}

// hasHan 字符串里有没有汉字（含扩展 A 区）。全角字母数字与全角标点不算——
// 代填值里出现全角冒号、全角空格不算「填了中文」，拦它们就过了。
func hasHan(s string) bool {
	for _, r := range s {
		if unicode.Is(unicode.Han, r) {
			return true
		}
	}
	return false
}

// warnNoShellDropsSudo --no-shell 与 sudo 同时生效时的提醒门（默认 y，回车即继续；
// 答 n 中止本次，让用户回命令行去掉 --no-shell 或 --sudo 重跑，不替他偷改参数）。
// 交互下先打一段说明再问一句；非交互（--yes）下由 Confirm 留痕 + 上屏一行。
func warnNoShellDropsSudo(in *common.Interactor, logger *log.Logger) error {
	if !in.Disinteractive {
		fmt.Printf("%s⚠ --sudo 与 --no-shell 同时给出%s\n", colorYellow, colorReset)
		fmt.Println("  --no-shell 是原样下发、工具不附加任何东西，所以 --sudo 不生效：")
		fmt.Println("  本次将以【登录用户】身份执行，命令里不会带 sudo。")
		fmt.Println("  要 root 身份，请去掉 --no-shell；要原样跑，请去掉 --sudo。")
	}
	ok, err := in.Confirm(common.ConfirmReq{
		Prompt:     "\n" + colorYellow + "继续执行？" + colorReset,
		DefaultYes: true,
		Trace:      "--sudo 被 --no-shell 忽略：本次以登录用户身份执行，命令里不含 sudo",
		SkipLine:   fmt.Sprintf("%s⚠ --sudo 被 --no-shell 忽略：本次以登录用户身份执行，命令里不含 sudo（要 root 请去掉 --no-shell）%s\n", colorYellow, colorReset),
	})
	if err != nil {
		return err
	}
	if !ok {
		fmt.Println(colorYellow + "操作已取消" + colorReset)
		logger.Warn("执行已取消（--no-shell 与 sudo 同时给出）")
		return common.ErrCancelled
	}
	return nil
}

// paramScreenText 渲染参数屏（横幅 + 信息表）。
//
// 打印侧直接 fmt.Print 这个返回值（与 cli.UsageText 同形：渲染成字符串，打印交给调用方）。
// 之所以要一个能拿到文本的口子：屏上每一行都是使用者可见面，得能被断言——
// 而 Confirm 要 *config.Config + *log.Logger + *common.Interactor，中途还会真实提问，
// 从它外面拿不到稳定文本。
func paramScreenText(args *cli.Args, nodes *nodelist.Nodes, highlight, note string) string {
	title := "           SSHFleet - 执行选项/参数确认           "
	border := strings.Repeat("═", len([]rune(title))+10)

	var b strings.Builder
	fmt.Fprintf(&b, "\n%s╔%s╗%s\n", colorCyan, border, colorReset)
	fmt.Fprintf(&b, "%s║  %s  ║%s\n", colorCyan, title, colorReset)
	fmt.Fprintf(&b, "%s╚%s╝%s\n\n", colorCyan, border, colorReset)
	b.WriteString(infoTableText(buildInfoTable(args, nodes, note), highlight))
	return b.String()
}

// buildInfoTable 构建显示信息的表格数据（行序与旧版一致）。
// note 非空时跟在并发数后面，让用户知道这个数从哪来（不指定 -n 的默认，或被建议改过）。
func buildInfoTable(args *cli.Args, nodes *nodelist.Nodes, note string) [][2]string {
	// 身份取 a.Identity().ScreenLabel()（判定与叫法都只此一份），不再在本函数里重判一次开关
	// ——2026-10-14 前这里与 batch 的日志各判各的，`--sudo --no-shell` 下两者说反话。
	// -c / -s 不出这一行（2026-10-28 作者定）：命令与脚本的身份要么与登录用户一致、
	// 要么被 --no-shell 剥掉——不生效的身份摆一行没有信息量，⚠ 提醒与日志里都有。
	var t [][2]string
	switch args.ModeName() {
	case cli.ModeCommand, cli.ModeScript:
	default:
		t = append(t, [2]string{"执行身份", args.Identity().ScreenLabel(args.NoShell)})
	}
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
		// 标签取「命令内容」而非「执行命令」：-c 收的是**一段命令文本**，
		// 「执行命令」既读得通「内容是这个」也读得通「去执行这个」，把文件名误敲给
		// -c 时扫一眼就过去了。与「脚本路径」「本地路径」同构后，一眼看得出这格是什么。
		t = append(t, modeLabel, [2]string{"命令内容", args.Command}, [2]string{"", ""})
	case cli.ModeScript:
		t = append(t, modeLabel, [2]string{"脚本路径", args.Script}, [2]string{"", ""})
	case cli.ModeUpload:
		t = append(t, modeLabel, [2]string{"本地路径", args.Upload}, [2]string{"远程路径", args.Path}, [2]string{"", ""})
	case cli.ModeDownload:
		t = append(t, modeLabel, [2]string{"远程路径", args.Download}, [2]string{"本地路径", args.Path}, [2]string{"", ""})
	case cli.ModePasswd:
		// 新密码是明确知道身份的凭据：按下确认之前看得见形态，但只看得到脱敏形态。
		// 取 NewPassword（解析后的明文）而不是 ChangePasswordRaw（旗标原值）：加密模式下
		// 旗标原值是一份密文文件的路径，脱敏出来是一串路径，屏上核不到这次到底要设什么。
		t = append(t, modeLabel, [2]string{"新密码", common.MaskSecret(args.NewPassword)}, [2]string{"", ""})
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
		// 名字表对「没选定模式」返零值——那就不加这一行，别摆个空标签（选项/参数合规已保证模式必选）。
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

// infoTableText 把信息表渲染成文本，对齐用 common.DisplayWidth（全角标点按 2 列计，不错位）。
// highlight 非空时，标签等于它的那一行加粗——用于"并发数被建议改了"的重显。
// 颜色不换：值一律亮橙，换色反而让那一行比别处暗（2026-10-07 作者定）。
func infoTableText(table [][2]string, highlight string) string {
	maxLabelWidth := 0
	for _, r := range table {
		if w := common.DisplayWidth(r[0]); w > maxLabelWidth {
			maxLabelWidth = w
		}
	}
	var b strings.Builder
	for _, r := range table {
		if r[0] == "" && r[1] == "" {
			b.WriteString("\n")
			continue
		}
		bold := ""
		if highlight != "" && r[0] == highlight {
			bold = colorBold
		}
		label := r[0] + strings.Repeat(" ", maxLabelWidth-common.DisplayWidth(r[0]))
		fmt.Fprintf(&b, "%s▶ %s-→%s   %s%s%s%s\n", colorBrightCyan, label, colorReset, bold, colorBrightOrange, r[1], colorReset)
	}
	return b.String()
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
	yes, err := in.Confirm(common.ConfirmReq{
		Prompt:     fmt.Sprintf("改成 %d 个？", allowed),
		DefaultYes: true,
		Trace:      fmt.Sprintf("非交互模式（--yes）自动采纳上传并发建议：%d 个（当前 %d 个）", allowed, args.Number),
	})
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

// uploadTreeTitle 上传内容清单的标题。
// 按规范口径称「选项」——`-x` 这类开关一律称选项，只有选项后面跟的值才叫参数（CHANGELOG 9.1.0）。
const uploadTreeTitle = "📁 上传文件/目录内容 (-u 选项):"

// showUploadContent 显示上传文件/目录内容（一层树，与旧版一致）。
// 读取失败返回错误交由 main 统一退出——不在子模块内自行 os.Exit（2026-09-14 审计修复）。
func showUploadContent(uPath string) error {
	text, err := uploadTreeText(uPath)
	if err != nil {
		return err
	}
	fmt.Print(text)
	return nil
}

// uploadTreeText 渲染上传内容清单。抽成能拿到文本的口子：这棵树是使用者可见面
// （他靠它核对「这次到底要传什么」），得能被断言；而 showUploadContent 直接写 stdout。
func uploadTreeText(uPath string) (string, error) {
	info, err := os.Stat(uPath)
	if err != nil {
		return "", fmt.Errorf("无法解析上传路径内容：%v\n请检查 -u 指定的路径是否存在且可访问", err)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "\n%s%s%s\n", colorYellow, uploadTreeTitle, colorReset)
	name := filepath.Base(uPath)
	if !info.IsDir() {
		fmt.Fprintf(&b, "└── %s (文件)\n", name)
		return b.String(), nil
	}
	fmt.Fprintf(&b, "└── %s/\n", name)
	entries, err := os.ReadDir(uPath)
	if err != nil {
		return "", fmt.Errorf("无法解析上传路径内容：%v\n请检查 -u 指定的路径是否存在且可访问", err)
	}
	for i, item := range entries {
		prefix := "    ├──"
		if i == len(entries)-1 {
			prefix = "    └──"
		}
		if item.IsDir() {
			fmt.Fprintf(&b, "%s %s/\n", prefix, item.Name())
		} else {
			fmt.Fprintf(&b, "%s %s\n", prefix, item.Name())
		}
	}
	return b.String(), nil
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
