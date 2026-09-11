// SSHFleet —— 批量 SSH 运维工具（单可执行文件，Go 重写版 5.0.0）
//
// main.go 承载「初始化 → 运行 → 退出」主干全流程（主干十步，见
// docs/issues/rewrite-to-go/spec.md 第六节）。所有环节的错误统一由
// main 打印并独占退出权；子模块只返回 error，不自行退出。
package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"sshfleet/internal/batch"
	"sshfleet/internal/cli"
	"sshfleet/internal/config"
	"sshfleet/internal/confirm"
	"sshfleet/internal/credential"
	"sshfleet/internal/dangercheck"
	"sshfleet/internal/log"
	"sshfleet/internal/nodelist"
	"sshfleet/internal/output"
	"sshfleet/internal/result"
)

// 配置文件位置：基准为当前工作目录（spec D28）。
const configPath = "./config/SSHFleet.conf"

// fatal 打印致命错误并退出（退出码 1）。main 独占退出权的唯一出口。
func fatal(where string, err error) {
	fmt.Fprintf(os.Stderr, "[ERROR] [function:%s] %s\n", where, err)
	os.Exit(1)
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

	// ---- 步骤 4：工具模式分流（keygen / convert-password）-------------
	// 独立工具与批量执行解耦，处理完直接退出，不进入后续步骤。
	if args.GenKey {
		logger.Info("进入密钥管理模式（--gen-key）")
		if err := credential.GenKey(args.Disinteractive); err != nil {
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
	if err := dangercheck.Check(args); err != nil {
		fatal("dangercheck", fmt.Errorf("危险关键词内容检查失败\n原因：%v", err))
	}
	logger.Success("危险关键词内容检查成功")

	// ---- 步骤 6：读取清单 + 字段补全 + 输入记忆 ------------------------
	nodes, err := nodelist.Read(args, cfg)
	if err != nil {
		fatal("nodelist", err)
	}
	logger.Success("读取节点信息成功")

	// ---- 步骤 7：参数确认（交互） --------------------------------------
	if err := confirm.Confirm(args, nodes, cfg); err != nil {
		fatal("confirm", err)
	}

	// ---- 步骤 8：并发执行 SSH / SFTP + 进度聚合 ------------------------
	execResults, err := batch.Run(args, cfg, nodes, logger)
	if err != nil {
		fatal("batch", err)
	}
	// ---- 步骤 9：结果统计 + 错误分类 -----------------------------------
	stats, err := result.Statistics(execResults, nodes, args)
	if err != nil {
		fatal("result", err)
	}

	// ---- 步骤 10：呈现 / 报告 / 归档 ------------------------------------
	if err := output.Render(stats, args, cfg); err != nil {
		fatal("output", err)
	}

	logger.Info(fmt.Sprintf("SSHFleet已退出，日志文件：%s", filepath.Join(cfg.Paths.Historys, cfg.Paths.Tool)))
}
