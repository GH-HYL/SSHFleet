// Package output 承载主干第 10 步：呈现 / 报告 / xlsx / 归档。
package output

import (
	"fmt"
	"os"

	"sshfleet/internal/batch"
	"sshfleet/internal/cli"
	"sshfleet/internal/config"
	"sshfleet/internal/log"
	"sshfleet/internal/result"
	"sshfleet/internal/verdict"
)

// Render 主干第 10 步：xlsx 导出 + 报告 + 资源备份 + 最新历史软链接。
// 终端统计打印由 main 在调用本函数前完成（顺序对位旧实现：统计 → 报告 → 资源 → xlsx）。
// 成败与分类读判定写好的结论（kw 只供统计提示与兜底判定用）。
func Render(
	archive *Archive,
	stats *result.Stats,
	results *batch.Results,
	a *cli.Args,
	cfg *config.Config,
	kw *verdict.Keywords,
	argv []string,
	logger *log.Logger,
) error {
	mode := result.ModeOf(a)

	if err := WriteReport(archive.Dir, stats, a, cfg, argv, kw); err != nil {
		return fmt.Errorf("格式化统计结果信息输出到报告文件失败\n原因：%v", err)
	}
	logger.Success("格式化统计结果信息输出到报告文件成功")

	if err := archive.BackupAssets(a); err != nil {
		return fmt.Errorf("保存执行资源文件失败\n原因：%v", err)
	}
	logger.Success("保存执行资源文件成功")

	// 两张 xlsx 的收尾同构（生成、失败只留痕不中断、成功报一句），共用一个出口；
	// 文件名统一取 config.BuiltinPaths，不写硬编码。
	writeXlsx(cfg.Enable.OutputToXlsx, config.BuiltinPaths.OutputXlsx, logger, func() error {
		return WriteOutputXlsx(archive.Dir, results, mode, a.Answers)
	})
	writeXlsx(cfg.Enable.ResultsToXlsx, config.BuiltinPaths.ResultsXlsx, logger, func() error {
		return WriteResultsXlsx(archive.Dir, results, mode, a.Answers)
	})

	if err := CreateLatestHistoryLink(); err != nil {
		fmt.Fprintf(os.Stderr, "%s[警告]%s %v\n", ansiYellow, ansiReset, err)
		logger.Warn(fmt.Sprintf("创建最新历史记录链接失败：%v", err))
	}
	return nil
}

// writeXlsx 一张 xlsx 的生成收尾：开关关闭即跳过；失败只留痕不中断——终端一关，
// 只有工具日志能说明「这次的 xlsx 为什么没生成」。
func writeXlsx(enabled bool, name string, logger *log.Logger, write func() error) {
	if !enabled {
		return
	}
	if err := write(); err != nil {
		msg := fmt.Sprintf("生成 %s 失败：%v（本次跳过，不影响执行结果）", name, err)
		fmt.Fprintf(os.Stderr, "%s[ERROR]%s %s\n", ansiRed, ansiReset, msg)
		logger.Error(msg)
		return
	}
	logger.Success(fmt.Sprintf("生成 %s 成功", name))
}
