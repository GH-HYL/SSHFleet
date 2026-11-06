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

	"sshfleet2/internal/archive"
	"sshfleet2/internal/console"
	"sshfleet2/internal/contract"
	"sshfleet2/internal/mask"
	"sshfleet2/internal/message"
)

// Run 递第三张纸与本次的落点，出屏幕统计块、写 `report.txt`，收尾补上「最新一次」链接。
//
// 次序照老版：先打统计块，再写文件。反过来的话，写文件失败时屏幕上什么都没有，
// 使用者只能看到一句报错、看不到这次跑成什么样。
//
// 交回的 warnings 是「打过了、还要记进工具日志」的那几句（今天只有链接那两句）：报告站自己
// 上屏（ADR-0032 决定四把「报告站上屏」列为四处之一），留痕交塔尖——它不持日志句柄。
func Run(l contract.Ledger, paths archive.Paths) (warnings []message.Line, err error) {
	// 解不开就是封与解两处漂了：直接报，不猜、不拿别处的值顶（15 号单「运行上下文的用法」）。
	body, err := openRunMeta(l.RunMeta)
	if err != nil {
		return nil, err
	}

	// 遮在报告站这一侧做，两处产出共用同一份已遮的值：贴出去的命令行与清单只有一个版。
	maskedArgv := mask.CommandLine(body.Argv)
	maskedManifest := mask.InlineRecordIf(l.Resources.Manifest)
	// 新密码印之前自己遮：密文里装的是明文（遮秘密不产生字段，谁要谁现算）。
	maskedPassword := mask.Secret(body.NewPassword)

	console.Print(message.StatisticsLine(l, body.ShowCategoryTips, body.ErrorKeywordsPath))

	if err := os.WriteFile(paths.Report, []byte(message.ReportText(l, body, maskedArgv, maskedManifest, maskedPassword)), 0o644); err != nil {
		return nil, fmt.Errorf("写报告文件失败（%s）：%w", paths.Report, err)
	}

	// 收尾：当前目录下那条入口指到这一次。建不起来与「撞上同名文件」都不算失败。
	if skipped, lerr := archive.CreateLatestLink(contract.HistoryDir); lerr != nil {
		w := message.LatestLinkFailed(lerr)
		console.Print(w)
		warnings = append(warnings, w)
	} else if skipped {
		w := message.LatestLinkSkipped()
		console.Print(w)
		warnings = append(warnings, w)
	}
	return warnings, nil
}
