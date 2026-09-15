// SSHFleet —— 批量 SSH 运维工具（单可执行文件，Go 重写版 5.0.0）
//
// main.go 承载「初始化 → 运行 → 退出」主干全流程（主干十步，见
// docs/issues/rewrite-to-go/spec.md 第六节）。所有环节的错误统一由
// main 打印并独占退出权；子模块只返回 error，不自行退出。
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"runtime/debug"
	"strings"
	"sync"
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
	"sshfleet/internal/ssh"
)

// 版本号：单一出处（显示在帮助信息首行下方，经 cli.Parse 传入 Usage）。
const appVersion = "5.0.1"

// versionWithBuildTime 版本号拼上编译来源时间（git HEAD 提交时间，go build 在
// 仓库内编译时自动注入 vcs.time）；取不到（仓库外编译 / -buildvcs=false）时只显示版本号。
func versionWithBuildTime() string {
	if bi, ok := debug.ReadBuildInfo(); ok {
		for _, s := range bi.Settings {
			if s.Key == "vcs.time" {
				if tm, err := time.Parse(time.RFC3339, s.Value); err == nil {
					return fmt.Sprintf("%s (%s)", appVersion, tm.Local().Format("2006-01-02 15:04:05"))
				}
			}
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

// fatal 打印致命错误并退出（退出码 1）。main 独占退出权的唯一出口。
// 交互取消（ErrCancelled）的文案已由交互器打印，此处只退出不再附加前缀。
func fatal(where string, err error) {
	if errors.Is(err, common.ErrCancelled) {
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "%s[ERROR]%s%s [function:%s]%s %s\n", colorRed, colorReset, colorYellow, where, colorReset, err)
	os.Exit(1)
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
	// 执行分界符：区分同一天多次执行，直接落盘（不经 logger，避免时间戳前缀）
	logger.Raw("\n    " + strings.Repeat("─", 50) + "\n\n")
	logger.Info("SSHFleet工具开始执行")
	logger.Info(fmt.Sprintf("工作路径：%s", func() string { wd, _ := os.Getwd(); return wd }()))
	logger.Info(fmt.Sprintf("原始命令行参数：%v", os.Args))
	logger.Info(fmt.Sprintf("日志目录：%s，工具日志文件名：%s", cfg.Paths.Historys, cfg.Paths.Tool))

	// ---- 步骤 3：解析命令行 ------------------------------------------
	args, err := cli.Parse(cfg, versionWithBuildTime(), os.Args[1:])
	if err != nil {
		if errors.Is(err, cli.ErrHelp) {
			return // 帮助已打印，以 0 退出
		}
		fatal("cli", fmt.Errorf("参数解析失败\n原因：%v", err))
	}
	logger.Success(fmt.Sprintf("参数解析成功，解析结果：%+v", args))

	// 交互器：全工具唯一的用户交互入口（In/Out 注入 + 非交互标志）
	in := common.NewInteractor(args.Disinteractive)

	// ---- 步骤 4：工具模式分流（keygen / convert-password）-------------
	// 独立工具与批量执行解耦，处理完直接退出，不进入后续步骤。
	if args.GenKey {
		logger.Info("进入密钥管理模式（--gen-key）")
		if err := credential.GenKey(in); err != nil {
			fatal("credential", err)
		}
		return
	}
	if args.ConvertPassword != "" {
		logger.Info("进入凭据转换模式（--convert-password）")
		if err := credential.ConvertPassword(args.ConvertPassword, cfg.Account.SecretDir, cfg.Account.PasswordSecurity); err != nil {
			fatal("credential", err)
		}
		return
	}

	// ---- 步骤 5：参数合规检查 + 危险命令检测 ---------------------------
	if err := cli.CheckConfigFiles(cfg); err != nil {
		fatal("cli", err)
	}
	if err := cli.CheckArguments(args); err != nil {
		fatal("cli", fmt.Errorf("参数合规性检查失败\n原因：%v", err))
	}
	logger.Success("输入的参数合规性检查成功")

	// 危险命令检测：规则文件加载（含规则校验）+ 命中判定 + 处置
	dangerRules, err := dangercheck.LoadRules(cfg.Paths.DangerousKeywords)
	if err != nil {
		fatal("dangercheck", fmt.Errorf("危险关键词内容检查失败\n原因：%v", err))
	}
	errorKeywords, err := result.LoadKeywords(cfg.Paths.ErrorKeywords)
	if err != nil {
		fatal("result", err)
	}
	dangerReport, err := dangercheck.Check(args, dangerRules)
	if err != nil {
		fatal("dangercheck", fmt.Errorf("危险关键词内容检查失败\n原因：%v", err))
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
	logger.Success("危险关键词内容检查成功")

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
	execLogPath := filepath.Join(archive.Dir, cfg.Paths.Exec)
	logger.Info(fmt.Sprintf("执行期日志已轮转至：%s（执行结束自动切回）", execLogPath))

	mode := result.ModeOf(args)
	if dangerNote != "" {
		execLog.Warn(dangerNote)
	}

	categoryOf := func(r ssh.Result) string {
		return result.Classify(result.Case{
			ExitCode:     r.ExitCode,
			Error:        errorText(r.Error),
			Output:       r.Output,
			AuthFailure:  errorText(r.AuthFailure),
			Mode:         mode,
			SuccessFiles: r.SuccessFiles,
			FailedFiles:  r.FailedFiles,
		}, errorKeywords)
	}

	outputPath := filepath.Join(archive.Dir, cfg.Paths.Output)
	outputFile, err := os.Create(outputPath)
	if err != nil {
		logger.Warn(fmt.Sprintf("无法创建 output.txt 文件: %v", err))
	} else {
		defer func() { _ = outputFile.Close() }()
	}

	// 进度界面延迟到首个进度事件才创建：采集期提示（如上传源中被过滤的软链接）
	// 得以先落到终端，不会被进度条的光标上移重绘覆盖（用户 2026-09-14 裁定）。
	// 界面创建后，运行期提示与单条结果改从界面上方打印（对位旧 rich Live：
	// 上部滚动输出、下部进度条，两块区域互不覆盖）。
	var (
		uiMutex sync.Mutex
		ui      *output.ProgressUI
	)
	printAbove := func(text string) {
		uiMutex.Lock()
		defer uiMutex.Unlock()
		if ui != nil {
			ui.PrintAbove(text)
			return
		}
		fmt.Fprintln(os.Stdout, text)
	}
	// batch 的运行期日志（开始执行任务）写执行期日志——此刻已轮转，不再进工具日志
	execResults, err := batch.Run(execCtx, args, cfg, nodes, execLog, batch.Hooks{
		OnNotice: func(msg string) {
			printAbove(msg)
			execLog.Info(msg)
		},
		OnProgress: func(s batch.Snapshot) {
			uiMutex.Lock()
			if ui == nil {
				ui = output.NewProgressUI(os.Stdout, mode, nodes.Len())
			}
			uiMutex.Unlock()
			ui.Update(s)
		},
		OnResult: func(r ssh.Result) {
			category := categoryOf(r)
			line := output.ResultLine(r, mode, category)
			// 终端明细改经 printAbove（进度界面上方），PrintResult 只负责 output.txt
			_ = output.PrintResult(io.Discard, outputFile, r, mode, category)
			if mode == "execute" {
				printAbove(line)
			}
			logNodeResult(execLog, r, mode, category)
		},
	})
	if ui != nil {
		ui.Stop()
	}
	logger.Info(fmt.Sprintf("执行期日志已写完，切回工具日志：%s", execLogPath))

	// 执行期日志收尾：连接与成败统计（对位旧引擎的「连接统计」「执行完成」记录）
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
	execLog.Info(fmt.Sprintf("连接统计：成功 %d，失败 %d", connOK, connFail))
	execLog.Info(fmt.Sprintf("执行结束：成功 %d 台，失败 %d 台", okCount, failCount))

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
	output.PrintStatistics(os.Stdout, stats, errorKeywords)

	// ---- 步骤 10：呈现 / 报告 / xlsx / 归档 -----------------------------
	if err := output.Render(archive, stats, execResults, args, cfg, errorKeywords, os.Args, categoryOf, logger); err != nil {
		fatal("output", err)
	}

	logger.Info(fmt.Sprintf("SSHFleet已退出，日志文件：%s", filepath.Join(cfg.Paths.Historys, cfg.Paths.Tool)))
	logger.Raw("\n    " + strings.Repeat("─", 50) + "\n\n")
}

// logNodeResult 把单节点结果按运行事件写入执行期日志（对位旧引擎的
// 「SSH连接成功/失败 → 执行结束/节点完成 → 分类」三级记录，全部带时间戳与级别）。
func logNodeResult(el *log.Logger, r ssh.Result, mode, category string) {
	ip := "【" + r.IP + "】"
	if r.ConnectSuccess {
		el.Success(fmt.Sprintf("%s连接成功，耗时 %.3fs", ip, r.ConnectCostTime))
	} else {
		errMsg := "未知错误"
		if r.Error != nil && *r.Error != "" {
			errMsg = *r.Error
		}
		el.Error(fmt.Sprintf("%s连接失败：%s", ip, errMsg))
	}

	if r.ConnectSuccess {
		switch mode {
		case "upload":
			if r.FailedFiles == 0 {
				el.Success(fmt.Sprintf("%s上传完成：成功 %d/%d 个文件", ip, r.SuccessFiles, r.TotalFiles))
			} else {
				el.Warn(fmt.Sprintf("%s上传完成：成功 %d/%d 个文件（有失败项）", ip, r.SuccessFiles, r.TotalFiles))
			}
		case "download":
			if r.FailedFiles == 0 {
				el.Success(fmt.Sprintf("%s下载完成：成功 %d/%d 个文件", ip, r.SuccessFiles, r.TotalFiles))
			} else {
				el.Warn(fmt.Sprintf("%s下载完成：成功 %d/%d 个文件（有失败项）", ip, r.SuccessFiles, r.TotalFiles))
			}
		default:
			if r.ExitCode != nil && *r.ExitCode == 0 {
				el.Success(fmt.Sprintf("%s命令执行成功，退出码 0，耗时 %.3fs", ip, r.ExecCostTime))
			} else {
				code := "无"
				if r.ExitCode != nil {
					code = fmt.Sprintf("%d", *r.ExitCode)
				}
				el.Error(fmt.Sprintf("%s命令执行失败，退出码 %s，耗时 %.3fs", ip, code, r.ExecCostTime))
			}
		}
	}
	el.Info(fmt.Sprintf("%s分类: %s", ip, category))
}
