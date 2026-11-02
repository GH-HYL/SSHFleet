package report

// 报告站：跑完之后出两样——屏幕上那段统计块，与本次归档目录里的 `report.txt`。
//
// 它只做四件事：解开运行上下文、把该遮的遮掉、向 `message` 要那两段字、一处上屏一处落盘。
// 一个字的版面都不自己排（版面全在 `message`），一条路径都不自己拼（`report.txt` 的落点
// 由归档包发、塔尖转手）。
//
// 为什么它是唯一解得开运行上下文的地方：见 contract/runmeta.go 与 ADR-0035 决定九。

import (
	"fmt"
	"os"

	"sshfleet2/internal/console"
	"sshfleet2/internal/contract"
	"sshfleet2/internal/mask"
	"sshfleet2/internal/message"
)

// Run 递第三张纸与报告文件落点，出屏幕统计块、写 `report.txt`。
//
// 次序照老版：先打统计块，再写文件。反过来的话，写文件失败时屏幕上什么都没有，
// 使用者只能看到一句报错、看不到这次跑成什么样。
func Run(l contract.Ledger, reportPath string) error {
	// 解不开就是封与解两处漂了：直接报，不猜、不拿别处的值顶（15 号单「运行上下文的用法」）。
	body, err := openRunMeta(l.RunMeta)
	if err != nil {
		return err
	}

	// 遮在报告站这一侧做，两处产出共用同一份已遮的值：贴出去的命令行与清单只有一个版。
	maskedArgv := mask.CommandLine(body.Argv)
	maskedManifest := mask.InlineRecordIf(l.Resources.Manifest)

	console.Print(message.StatisticsLine(l, body.ShowCategoryTips, body.ErrorKeywordsPath))

	if err := os.WriteFile(reportPath, []byte(message.ReportText(l, body, maskedArgv, maskedManifest)), 0o644); err != nil {
		return fmt.Errorf("写报告文件失败（%s）：%w", reportPath, err)
	}
	return nil
}
