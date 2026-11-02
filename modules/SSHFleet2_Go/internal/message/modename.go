package message

// 模式名字表：一种模式的叫法分几列，一列一处成形。
//
// **预填充**：老版把五档模式的九列叫法收在 `cli.Mode.Info()`（ADR-0013：加一档模式原先要
// 逐处核对七个点、手改四处），新工程里 `cli` 劈进了解析与闸门、模式类型住契约，而契约不许
// 有函数——所以这张表落在 `message`：它是唯一改字处，ADR-0013 要的「名字只改一处」照样成立。
//
// 眼下只搬了要被两个以上站同时读到的两列：
//
//	成功分类  执行站写进纸、结算站比对、报告站印（11 号单）
//	动作名    结果行前半截「执行: 成功 - 0.123s」（12 号单）
//
// 其余七列（归档目录名、报告标题、模式行、超时行措辞……）随 05／07／15 各自要用时再并进来。
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

// actionName 本模式那个动作叫什么：结果行前半截（`执行: 成功 - 0.123s`）用它。
//
// 命令与脚本共用一个「执行」——老版名字表里这两档本来就同值，不是这里合并的。
func actionName(m contract.Mode) string {
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
