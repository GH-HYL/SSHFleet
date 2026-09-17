// SSHFleet —— 批量 SSH 运维工具（单可执行文件，Go 重写版 5.0.0）
//
// main.go 承载「初始化 → 运行 → 退出」主干全流程。
// 所有环节的错误统一由 main 打印并独占退出权；子模块只返回 error，不自行退出。
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"runtime/debug"
	"strings"
	"syscall"
	"time"

	"sshfleet/internal/batch"
	"sshfleet/internal/cli"
	"sshfleet/internal/common"
	"sshfleet/internal/config"
	"sshfleet/internal/confirm"
	"sshfleet/internal/credential"
	"sshfleet/internal/dangercheck"
	"sshfleet/internal/log"
	"sshfleet/internal/nodelist"
	"sshfleet/internal/output"
	"sshfleet/internal/result"
)

// 版本号：单一出处（显示在帮助信息首行下方，经 cli.Parse 传入 Usage）。
// 与 CHANGELOG 顶部当天段落的段头**同一个号**——开段、抬号时在同一次提交里同步改，
// 两处不一致即为错误。同一天的改动共用一个号，不因改动多而另起号。
const appVersion = "5.2.1"

// versionWithBuildID 版本号拼上**构建标识**：git 短提交号（仓库内编译时由 go build
// 自动注入 vcs.revision；工作区有未提交改动时加 -dirty）+ HEAD 提交时间。
//
// 为什么要有构建标识：同一天可能编译多次，版本号是同一个，光看 v5.1.0 分不清是哪一次
// 代码。让人报出这一整行（如 v5.1.0+706036d (2026-09-15 10:43:52)）就能定位到那次提交。
// 取不到（仓库外编译 / -buildvcs=false）时退化为只显示版本号。
func versionWithBuildID() string {
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return appVersion
	}
	var rev, tm string
	dirty := false
	for _, s := range bi.Settings {
		switch s.Key {
		case "vcs.revision":
			rev = s.Value
		case "vcs.time":
			tm = s.Value
		case "vcs.modified":
			dirty = s.Value == "true"
		}
	}
	id := ""
	if rev != "" { // 短提交号取前 7 位（与 git 的默认缩写一致）
		id = rev
		if len(id) > 7 {
			id = id[:7]
		}
		if dirty {
			id += "-dirty"
		}
	}
	switch {
	case id != "" && tm != "":
		if t, err := time.Parse(time.RFC3339, tm); err == nil {
			return fmt.Sprintf("%s+%s (%s)", appVersion, id, t.Local().Format("2006-01-02 15:04:05"))
		}
		return fmt.Sprintf("%s+%s", appVersion, id)
	case tm != "":
		if t, err := time.Parse(time.RFC3339, tm); err == nil {
			return fmt.Sprintf("%s (%s)", appVersion, t.Local().Format("2006-01-02 15:04:05"))
		}
	}
	return appVersion
}

// 配置文件位置：基准为当前工作目录（spec D28）。
const configPath = "./config/SSHFleet.conf"

// 危险命令确认提示与 [ERROR] 前缀的配色（对位旧 constants.py / error_handler.py）。
const (
	colorReset  = "\x1b[0m"
	colorRed    = "\x1b[31m"
	colorYellow = "\x1b[33m"
)

// toolLog / execLogRef 是 fatal 写日志用的两个落点：工具日志在步骤 2 建好之后赋值，
// 执行期日志在步骤 8 建好之后赋值。都可能是 nil——配置加载失败或初始化日志失败时
// 两个都还没有（那时连日志路径都还没确定，只能落 stderr）。
var (
	toolLog    *log.Logger
	execLogRef *log.Logger
)

// fatal 打印致命错误并退出（退出码 1）。main 独占退出权的唯一出口。
// 交互取消（ErrCancelled）的文案已由交互器打印，此处只退出不再附加前缀。
//
// 致命错误同时落日志：终端一关就只剩日志可查——此前这类错误只打 stderr，
// 日志里一片空白，事后完全查不出「那次为什么没跑起来」。
func fatal(where string, err error) {
	if errors.Is(err, common.ErrCancelled) {
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "%s[ERROR]%s%s [function:%s]%s %s\n", colorRed, colorReset, colorYellow, where, colorReset, err)
	closeLogsAfterFatal(where, err)
	os.Exit(1)
}

// closeLogsAfterFatal 把致命错误逐行写入工具日志与执行期日志（若有），并关闭句柄。
// os.Exit 不跑 defer，所以这里显式落盘收尾。
func closeLogsAfterFatal(where string, err error) {
	for _, lg := range []*log.Logger{toolLog, execLogRef} {
		if lg == nil {
			continue
		}
		logErrorLines(lg, where, err)
		_ = lg.Close()
	}
}

// logErrorLines 把可能多行的错误文本逐行写日志：每条日志一行、格式与其他行一致，
// 续行缩进两格（错误文案本身就带「原因：…」「请检查…」这类换行分段）。
func logErrorLines(lg *log.Logger, where string, err error) {
	first := true
	for _, line := range strings.Split(strings.ReplaceAll(err.Error(), "\r\n", "\n"), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if first {
			lg.Error(fmt.Sprintf("[%s] %s", where, line))
			first = false
			continue
		}
		lg.Error(fmt.Sprintf("[%s]   %s", where, line))
	}
}

// errorText 解引用错误指针（空指针给空串）。
func errorText(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func main() {
	// ---- 步骤 1：加载配置 --------------------------------------------
	cfg, err := config.Load(configPath)
	if err != nil {
		fatal("config", fmt.Errorf("加载配置文件失败：%s\n原因：%v\n请检查该文件是否存在、TOML 格式是否正确后重试", configPath, err))
	}

	// ---- 步骤 2：初始化日志 ------------------------------------------
	logger, err := log.Init(cfg.Paths.Historys, cfg.Paths.Tool)
	if err != nil {
		fatal("log", fmt.Errorf("初始化工具日志失败：%v\n请检查配置 paths.historys 指向的日志目录是否存在且可写，然后重试", err))
	}
	defer func() { _ = logger.Close() }()
	toolLog = logger // 交给 fatal：此后任何致命错误都要落日志
	// 执行分界符：区分同一天多次执行，直接落盘（不经 logger，避免时间戳前缀）
	logger.Raw("\n    " + strings.Repeat("─", 50) + "\n\n")
	logger.Info("SSHFleet工具开始执行")
	logger.Info(fmt.Sprintf("工作路径：%s", func() string { wd, _ := os.Getwd(); return wd }()))
	logger.Info(fmt.Sprintf("原始命令行参数：%v", os.Args))
	logger.Info(fmt.Sprintf("日志目录：%s，工具日志文件名：%s", cfg.Paths.Historys, cfg.Paths.Tool))

	// ---- 步骤 3：解析命令行 ------------------------------------------
	args, err := cli.Parse(cfg, versionWithBuildID(), os.Args[1:])
	if err != nil {
		if errors.Is(err, cli.ErrHelp) {
			return // 帮助已打印，以 0 退出
		}
		fatal("cli", fmt.Errorf("参数解析失败\n原因：%v", err))
	}
	logger.Success(fmt.Sprintf("参数解析成功，解析结果：%+v", args))

	// 交互器：全工具唯一的用户交互入口（In/Out 注入 + 非交互标志）
	in := common.NewInteractor(args.Disinteractive)

	// ---- 步骤 4：工具模式分流（keygen / key-status / convert-password）-----
	// 独立工具与批量执行解耦，处理完直接退出，不进入后续步骤。
	if args.GenKey {
		logger.Info("进入密钥管理模式（--gen-key）")
		if err := credential.GenKey(in); err != nil {
			fatal("credential", err)
		}
		return
	}
	if args.KeyStatus {
		logger.Info("进入主密钥状态查看模式（--key-status）")
		fmt.Print(credential.KeyStatusReport())
		return
	}
	if args.ConvertPassword != "" {
		logger.Info("进入凭据转换模式（--convert-password）")
		if err := credential.ConvertPassword(args.ConvertPassword, cfg.Account.SecretDir, cfg.Account.PasswordSecurity); err != nil {
			fatal("credential", err)
		}
		return
	}

	// ---- 步骤 4.5：主密钥预检查 ---------------------------------------
	// 「生成密钥后忘了 source / 重开终端」是最高频的坑（用户 2026-09-15 指出）：
	// 与其等他用凭据时撞一句看不懂的报错，不如开工前就把现状与下一步说清。
	if note := credential.PrecheckKey(); note != "" {
		state, _ := credential.InspectKey()
		logger.Warn(fmt.Sprintf("主密钥预检查未通过（%s）", state))
		fmt.Fprintf(os.Stderr, "\n%s[警告]%s %s", colorYellow, colorReset, note)
	}

	// ---- 步骤 5：参数合规检查 + 危险命令检测 ---------------------------
	if err := cli.CheckConfigFiles(cfg); err != nil {
		fatal("cli", err)
	}
	if err := cli.CheckArguments(args); err != nil {
		fatal("cli", fmt.Errorf("参数合规性检查未通过\n原因：%v", err))
	}
	logger.Success("输入的参数合规性检查通过")

	// 危险命令检测：规则文件加载（含规则校验）+ 命中判定 + 处置
	dangerRules, err := dangercheck.LoadRules(cfg.Paths.DangerousKeywords)
	if err != nil {
		fatal("dangercheck", fmt.Errorf("危险关键词内容检查未通过\n原因：%v", err))
	}
	errorKeywords, err := result.LoadKeywords(cfg.Paths.ErrorKeywords)
	if err != nil {
		fatal("result", err)
	}
	dangerReport, err := dangercheck.Check(args, dangerRules)
	if err != nil {
		fatal("dangercheck", fmt.Errorf("危险关键词内容检查未通过\n原因：%v", err))
	}
	dangerNote := "" // 危险命令放行留痕（spec D35/D46）：先写工具日志，归档后同样写入执行日志
	switch {
	case dangerReport.HasForbidden():
		// forbidden：打印警告后直接退出（与旧行为一致）
		output.PrintDangerWarning(dangerReport, true)
		logger.Warn(fmt.Sprintf("命中禁止命令，已退出：%s", dangerReport.Matches[0].Content))
		_ = logger.Close()
		os.Exit(1)
	case len(dangerReport.Matches) > 0 && in.Disinteractive:
		// 非交互模式：非 forbidden 放行，但写入工具日志留痕（spec D35）
		dangerNote = fmt.Sprintf("非交互模式放行危险命令（最高级别 %s，分类 %s，来源行 %d）：%s",
			dangerReport.Highest(), dangerReport.Matches[0].RuleName, dangerReport.Matches[0].Line, dangerReport.Matches[0].Content)
		logger.Warn(dangerNote)
	case len(dangerReport.Matches) > 0:
		output.PrintDangerWarning(dangerReport, false)
		confirmed, cerr := in.Confirm("\n"+colorYellow+"已明确风险继续执行？"+colorReset, false)
		if cerr != nil {
			fatal("dangercheck", cerr)
		}
		if !confirmed {
			fmt.Println(colorYellow + "操作已取消" + colorReset)
			logger.Warn("执行已取消，SSHFleet工具已退出")
			// 取消属非致命终止：与 confirm 确认取消同语义，以退出码 1 结束（2026-09-14 裁定，
			// 对齐旧版 dangerous.py 取消即 sys.exit(1) 的行为）。fatal 对 ErrCancelled
			// 不加 [ERROR] 前缀、直接以 1 退出。
			_ = logger.Close()
			fatal("dangercheck", common.ErrCancelled)
		}
		dangerNote = fmt.Sprintf("用户已确认风险继续执行（最高级别 %s）：%s", dangerReport.Highest(), dangerReport.Matches[0].Content)
		logger.Warn(dangerNote)
	}
	logger.Success("危险关键词内容检查通过")

	// ---- 步骤 6：读取清单 + 字段补全 + 输入记忆 ------------------------
	nodes, err := nodelist.Read(args, cfg, in)
	if err != nil {
		fatal("nodelist", err)
	}
	logger.Success("读取节点信息成功")

	// ---- 步骤 7：参数确认（交互） --------------------------------------
	if err := confirm.Confirm(args, nodes, cfg, logger, in); err != nil {
		fatal("confirm", err)
	}

	// ---- 步骤 8：并发执行 SSH / SFTP + 进度聚合 ------------------------
	// 中断：信号 → 取消 context → worker 停止启动新任务；已完成的节点结果照常进入后续整理
	execCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	execStart := time.Now()

	// 归档目录：本次执行的全部产物（执行日志 / 终端输出 / 报告 / xlsx / 资源备份）
	archive, err := output.CreateArchive(cfg, args)
	if err != nil {
		fatal("output", err)
	}
	logger.Success(fmt.Sprintf("生成归档目录成功: %s", archive.Dir))

	// 执行期日志（对位旧引擎写入归档目录的 SSHFleet.log）：带时间戳与级别的
	// 节点级运行明细。执行开始即轮转过去，结束后切回工具日志并在此声明去向。
	execLog, err := log.InitExec(archive.Dir, cfg.Paths.Exec)
	if err != nil {
		fatal("log", fmt.Errorf("创建执行期日志失败\n原因：%v", err))
	}
	defer func() { _ = execLog.Close() }()
	execLogRef = execLog // 交给 fatal：批量执行阶段的致命错误也要落进本次执行的档案
	execLogPath := filepath.Join(archive.Dir, cfg.Paths.Exec)
	logger.Info(fmt.Sprintf("执行期日志已轮转至：%s（执行结束自动切回）", execLogPath))

	mode := result.ModeOf(args)
	if dangerNote != "" {
		execLog.Warn(dangerNote)
	}

	outputPath := filepath.Join(archive.Dir, cfg.Paths.Output)
	outputFile, err := os.Create(outputPath)
	if err != nil {
		logger.Warn(fmt.Sprintf("无法创建 output.txt 文件: %v", err))
	} else {
		defer func() { _ = outputFile.Close() }()
	}

	// 单节点结果的三个去向（终端明细 / output.txt / 执行期日志）与进度界面的
	// 懒创建、上打提示都收在 output 的呈现器里；main 只构造并接上 batch 的三个事件。
	// （呈现器只做呈现，不控制生命周期——主干与退出权仍在本函数手里。）
	reporter := output.NewReporter(execLog, outputFile, mode, nodes.Len(), errorKeywords, execStart)
	// batch 的运行期日志（开始执行任务）写执行期日志——此刻已轮转，不再进工具日志
	execResults, err := batch.Run(execCtx, args, cfg, nodes, execLog, batch.Hooks{
		OnNotice:   reporter.Notice,
		OnProgress: reporter.Progress,
		OnResult:   reporter.Result,
	})
	reporter.Stop()
	logger.Info(fmt.Sprintf("执行期日志已写完，切回工具日志：%s", execLogPath))

	// 汇总本轮连接与成败（写入执行期日志的收尾行）
	var connOK, connFail, okCount, failCount int
	for _, r := range execResults.Items {
		if r.ConnectSuccess {
			connOK++
		} else {
			connFail++
		}
		if r.ExitCode != nil && *r.ExitCode == 0 {
			okCount++
		} else {
			failCount++
		}
	}
	// 执行期日志收尾：连接与成败统计合成一行（对位旧引擎的「连接统计」「执行完成」两条，
	// 正常路径能合并就合并——1000+ 节点时日志要尽量短）。
	execLog.Info(fmt.Sprintf("执行结束：成功 %d 台，失败 %d 台；连接成功 %d，连接失败 %d",
		okCount, failCount, connOK, connFail))

	if err != nil {
		fatal("batch", err)
	}
	if execCtx.Err() != nil {
		fmt.Println("已收到中断信号，SSHFleet 停止执行（已完成节点的结果已写入日志/输出文件）")
		logger.Warn("收到中断信号，执行已停止")
	}
	// ---- 步骤 9：结果统计 + 错误分类 -----------------------------------
	stats := result.Statistics(execResults, nodes, args, errorKeywords, execStart, time.Now())
	logger.Success("计算统计结果信息成功")
	output.PrintStatistics(os.Stdout, stats, errorKeywords, cfg.Enable.ShowCategoryTips)

	// ---- 步骤 10：呈现 / 报告 / xlsx / 归档 -----------------------------
	if err := output.Render(archive, stats, execResults, args, cfg, errorKeywords, os.Args, reporter.Category, logger); err != nil {
		fatal("output", err)
	}

	logger.Info(fmt.Sprintf("SSHFleet已退出，日志文件：%s", filepath.Join(cfg.Paths.Historys, cfg.Paths.Tool)))
	logger.Raw("\n    " + strings.Repeat("─", 50) + "\n\n")
}
