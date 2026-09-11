// Package output 承载主干第 10 步：呈现 / 报告 / 归档（M5 落地）。
//
// 进度渲染（终端进度条、速度显示）住这里（spec D2 展开）；归档目录两个
// 日志文件按用途命名 tool / exec（D36）；report.txt 补齐下载模式参数段（D37）；
// assets/ 只备份清单与脚本（D38）。
package output

import (
	"fmt"

	"sshfleet/internal/cli"
	"sshfleet/internal/config"
	"sshfleet/internal/result"
)

// errNotImplemented 占位：M5 落地后删除。
var errNotImplemented = fmt.Errorf("未实现：M5 输出层落地")

// Render 呈现统计结果并生成报告 / xlsx / 归档。
func Render(stats *result.Stats, a *cli.Args, cfg *config.Config) error {
	_, _, _ = stats, a, cfg
	return errNotImplemented
}
