package message

// 模式名字表：一种模式的叫法分几列，一列一处成形。
//
// **预填充**：老版把五档模式的九列叫法收在 `cli.Mode.Info()`（ADR-0013：加一档模式原先要
// 逐处核对七个点、手改四处），新工程里 `cli` 劈进了解析与闸门、模式类型住契约，而契约不许
// 有函数——所以这张表落在 `message`：它是唯一改字处，ADR-0013 要的「名字只改一处」照样成立。
//
// 眼下搬了这几列：
//
//	成功分类  执行站写进纸、结算站比对、报告站印（11 号单）
//	动作名    结果行前半截「执行: 成功 - 0.123s」（12 号单）
//	模式名    确认屏与报告「执行模式：」那一行（15 号单）
//	超时行措辞 报告「执行超时／传输超时／改密超时」（15 号单）
//
// 其余几列（归档目录名、报告标题之外的那几处叫法……）随 05／07 各自要用时再并进来。
// 并的时候整张表一处成形，别处仍不许写字。

import "sshfleet2/internal/contract"

// SuccessCategory 本模式「成功」那一类的名字：命令与脚本「执行成功」、
// 上传下载「传输成功」、改密「改密成功」。
func SuccessCategory(m contract.Mode) string {
	switch m {
	case contract.ModeCommand, contract.ModeScript:
		return "执行成功"
	case contract.ModeUpload, contract.ModeDownload:
		return "传输成功"
	case contract.ModePasswd:
		return "改密成功"
	}
	return ""
}

// ActionName 本模式那个动作叫什么：结果行前半截（`执行: 成功 - 0.123s`）与执行期日志都用它。
//
// 命令与脚本共用一个「执行」——老版名字表里这两档本来就同值，不是这里合并的。
func ActionName(m contract.Mode) string {
	switch m {
	case contract.ModeCommand, contract.ModeScript:
		return "执行"
	case contract.ModeUpload:
		return "上传"
	case contract.ModeDownload:
		return "下载"
	case contract.ModePasswd:
		return "改密"
	}
	return ""
}

// LogName 本模式在执行期日志里怎么称呼（「开始执行任务：… 模式 命令」那一行）。
func LogName(m contract.Mode) string {
	switch m {
	case contract.ModeCommand:
		return "命令"
	case contract.ModeScript:
		return "脚本"
	case contract.ModeUpload:
		return "上传"
	case contract.ModeDownload:
		return "下载"
	case contract.ModePasswd:
		return "改密"
	}
	return ""
}

// LogLabel 执行期日志里「…失败」那一行的主语：「命令执行」「脚本执行」。
//
// 只有命令与脚本用得上这一句——上传下载与改密在日志里另起句式（老版 `logNode` 里就是
// 分开的三支，那两支不取 LogName + ActionName），所以这里对其余模式给空串，缺的那几支
// 随阶段 05。给空串而不是硬拼：拼出「上传上传」比空着更难查。
func LogLabel(m contract.Mode) string {
	switch m {
	case contract.ModeCommand, contract.ModeScript:
		return LogName(m) + ActionName(m)
	}
	return ""
}

// modeTitle 本模式的正式叫法：确认屏「执行模式」那一格与 `report.txt` 同一行都用它
// （老版 `ScreenLabel`／`ReportTitle` 两列同值，这里并成一列）。
func modeTitle(m contract.Mode) string {
	switch m {
	case contract.ModeCommand:
		return "命令模式"
	case contract.ModeScript:
		return "脚本模式"
	case contract.ModeUpload:
		return "上传模式"
	case contract.ModeDownload:
		return "下载模式"
	case contract.ModePasswd:
		return "改密模式"
	}
	return ""
}

// timeoutLabel 报告里那个超时行怎么称呼：命令与脚本是执行超时、上传下载是传输超时、
// 改密是改密超时（老版 `TimeoutLabel`）。
func timeoutLabel(m contract.Mode) string {
	switch m {
	case contract.ModeCommand, contract.ModeScript, contract.ModePasswd:
		return "执行超时"
	case contract.ModeUpload, contract.ModeDownload:
		return "传输超时"
	}
	return ""
}
