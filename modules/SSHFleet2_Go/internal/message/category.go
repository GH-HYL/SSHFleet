package message

// 分类名：判据表给的那些由配置文件写，剩下三个由代码给——成功那一类、按退出码兜底的那一类、
// 完全无信息时的兜底。三个都是给人看的字，所以住这里（ADR-0032 第四条「唯一改字处」）。
//
// 三个站读的是同一份：执行站写进纸里、结算站拿它算成败、报告站印出来。分住三处就等于
// 同一句话有三个来源，改一个漏一个只剩静默对不上。

import (
	"strconv"

	"sshfleet2/internal/contract"
)

// CategoryUnknown 完全无信息时的兜底分类（老版 `verdict.Unclassified`，一字不改）。
const CategoryUnknown = "原因未知"

// exitCodeCategoryPrefix 按退出码自动给的兜底分类的前缀。
//
// 那个名字里带一个数字，配置文件写不了这样的 `name`，所以它是本工程唯一一处「代码给分类名」
// （ADR-0021 决定第 3 条的例外，老版 `verdict.exitCodeFailPrefix` 同口径）。
const exitCodeCategoryPrefix = "执行失败(退出码"

// CategoryOfExitCode 目的命令以非 0 收场、判据表第二组也没认出来时按退出码给的那一类。
//
// 名字是「执行失败(退出码3)」这种形状。要那个数字时读纸上「退出码」那一格，**不要去解名字**
// ——解名字等于把一处字面量当成数据源（老版 2026-10-09 就为这个改过一次）。
func CategoryOfExitCode(code int) string {
	return exitCodeCategoryPrefix + strconv.Itoa(code) + ")"
}

// SuccessCategory 本模式「成功」那一类的名字：命令与脚本「执行成功」、上传下载「传输成功」、
// 改密「改密成功」。
//
// **预填充**：老版把一种模式的全部叫法收在 `cli.Mode.Info()`（ADR-0013），新工程里 `cli`
// 劈进了解析与闸门、模式类型住契约，而契约不许有函数——名字表暂时只搬「成功分类」这一列
// 住这里，因为这个阶段只有它要被三个站同时读到。其余七列（归档目录名、报告标题、模式行、
// 超时行措辞……）随 05／07／15 各自要用时再并进来，并的时候整张表一处成形，别处仍不许写字。
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
