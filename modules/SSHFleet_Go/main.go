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
	"os"
	"os/signal"
	"path/filepath"
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

// 配置文件位置：基准为当前工作目录（spec D28）。
const configPath = "./config/SSHFleet.conf"

// fatal 打印致命错误并退出（退出码 1）。main 独占退出权的唯一出口。
// 交互取消（ErrCancelled）的文案已由交互器打印，此处只退出不再附加前缀。
func fatal(where string, err error) {
	if errors.Is(err, common.ErrCancelled) {
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "[ERROR] [function:%s] %s\n", where, err)
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
	logger.Info("SSHFleet工具开始执行")
	logger.Info(fmt.Sprintf("工作路径：%s", func() string { wd, _ := os.Getwd(); return wd }()))
	logger.Info(fmt.Sprintf("原始命令行参数：%v", os.Args))
	logger.Info(fmt.Sprintf("日志目录：%s，工具日志文件名：%s", cfg.Paths.Historys, cfg.Paths.Tool))

	// ---- 步骤 3：解析命令行 ------------------------------------------
	args, err := cli.Parse(cfg, os.Args[1:])
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
		confirmed, cerr := in.Confirm("\n已明确风险继续执行？", false)
		if cerr != nil {
			fatal("dangercheck", cerr)
		}
		if !confirmed {
			fmt.Println("操作已取消")
			logger.Warn("执行已取消，SSHFleet工具已退出")
			return
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
	if dangerNote != "" {
		_ = archive.ExecLog("%s", dangerNote)
	}

	mode := result.ModeOf(args)
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

	ui := output.NewProgressUI(os.Stdout, mode, nodes.Len())
	execResults, err := batch.Run(execCtx, args, cfg, nodes, logger, batch.Hooks{
		OnProgress: ui.Update,
		OnResult: func(r ssh.Result) {
			category := categoryOf(r)
			_ = output.PrintResult(os.Stdout, outputFile, r, mode, category)
			_ = archive.ExecLog("%s", output.ResultLine(r, mode, category))
		},
	})
	ui.Stop()
	_ = archive.Close()
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
}
