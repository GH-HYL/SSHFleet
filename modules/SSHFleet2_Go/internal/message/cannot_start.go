package message

import (
	"strconv"

	"sshfleet2/internal/contract"
)

// 「起不来」的模板：每个类别一句，句子只在这里成形（ADR-0044）。
//
// 纸里只装类别与参数，站里不再自己拼一句塞进纸——要加一种说法，就得在这里加一条模板。
// 判据文件那一族的表格层（读不到／解析失败／缺节／未识别字段）三份表共用同一个类别，
// 条目层每句各不相同，所以各占一条。

// CannotStartText 是「起不来」那一屏的正文：按原因类别取模板成形。
//
// 日志与屏幕共用这一份（老版那两处本来同一个字符串），不各拼一遍——ADR-0028 第五节讲的
// 「屏给使用者看、日志留现场」讲的是形态，不是让两处各说一句（ADR-0044）。
//
// 迁移期（01／02 号单）：纸上有类别就走模板，没有就把 `Body` 原样交回；03 号单删掉
// `Body` 之后，前一条是唯一的路。
func CannotStartText(c contract.CannotStart) string {
	r := c.Reason
	if r.Kind == "" {
		return c.Body
	}
	text := reasonText(r)
	if r.Head == "" {
		return text
	}
	return r.Head + "：" + text
}

// reasonText 是模板本体：按类别排那一句，不带外壳句。
func reasonText(r contract.CannotStartReason) string {
	switch r.Kind {
	case contract.CannotStartConfigLoad:
		return "加载配置文件失败（" + r.Place + "）：" + r.Origin

	// 判据文件那一族的表格层：三份表共用。
	case contract.CannotStartTableRead:
		return r.Place + "失败：" + r.Value + reasonTail(r.Origin)
	case contract.CannotStartTableParse:
		return r.Place + "解析失败：" + r.Value + reasonTail(r.Origin)
	case contract.CannotStartTableEmpty:
		return r.Place + "里一条 " + r.Value + " 都没有：" + r.Origin
	case contract.CannotStartTableUnknownField:
		return r.Place + "含未识别字段：" + r.Value + originTail(r.Origin)
	case contract.CannotStartCategoriesEmpty:
		return "错误分类关键词为空：" + r.Place
	case contract.CannotStartPasswdEmptyStep:
		return "改密配置里一条 step 都没有：" + r.Origin + hintTail("至少留一条 [[step]]")
	case contract.CannotStartPasswdNoSignal:
		return "改密缺少入场信号" + reasonTail("判据文件里「密码过期」分类没有关键词，工具认不出哪台机器要改密") +
			hintTail("该分类在 config/keywords_error.conf，别删它")

	// 改密提示词表的条目层：前缀是「改密配置第 N 条 step：」。
	case contract.CannotStartPasswdStepNoKeywords:
		return passwdItem(r) + "缺少 keywords（提示原文）"
	case contract.CannotStartPasswdStepEmptyItem:
		return passwdItem(r) + "keywords 里有空项"
	case contract.CannotStartPasswdStepBadValue:
		return passwdItem(r) + "value 只能写 " + contract.PasswdValueCurrent + " 或 " +
			contract.PasswdValueNew + "，当前值：" + r.Value
	case contract.CannotStartPasswdStepBadMax:
		return passwdItem(r) + "max 必须是正整数，当前值：" + r.Value
	case contract.CannotStartPasswdCatNoName:
		return passwdItem(r) + "缺少 name（分类名）"
	case contract.CannotStartPasswdCatEmptyItem:
		return passwdItem(r) + r.Value + " 的 keywords 里有空项"

	// 错误分类表的条目层：这一处没有冒号，句子自成一句。
	case contract.CannotStartKeywordCatNoName:
		return "错误分类关键词第 " + strconv.Itoa(r.Index) + " 条缺少 name"
	case contract.CannotStartKeywordCatEmptyItem:
		return "错误分类关键词里 " + r.Value + " 的 keywords 有空项"

	// 危险规则表的条目层：前缀是「危险命令检测规则第 N 条：」。
	case contract.CannotStartDangerNoName:
		return dangerItem(r) + "缺少 name"
	case contract.CannotStartDangerBadLevel:
		return dangerItem(r) + "risk_level 只能写 " + contract.RiskForbidden + " / " +
			contract.RiskHigh + " / " + contract.RiskMedium + " / " + contract.RiskLow +
			"，当前值：" + r.Value
	case contract.CannotStartDangerExampleTooWide:
		return dangerItem(r) + r.Value + " 的 example 有 " + r.Origin + " 列宽，超过上限 " +
			strconv.Itoa(r.Limit) + " 列（一个汉字算两列）"
	case contract.CannotStartDangerReasonTooWide:
		return dangerItem(r) + r.Value + " 的 reason 有 " + r.Origin + " 列宽，超过上限 " +
			strconv.Itoa(r.Limit) + " 列（一个汉字算两列）"
	case contract.CannotStartDangerBadEnabled:
		return dangerItem(r) + r.Value + " 的 enabled 只认 true / false"
	case contract.CannotStartDangerNoRegex:
		return dangerItem(r) + r.Value + " 没有 regex"
	case contract.CannotStartDangerEmptyRegex:
		return dangerItem(r) + r.Value + " 的 regex 是空的"
	case contract.CannotStartDangerBadRegex:
		return dangerItem(r) + r.Value + " 的一条正则编译不过：" + r.Origin

	// 解析站其余三族里已经搬过来的几条（03 号单）。
	case contract.CannotStartExtraArgs:
		return RefuseExtraArgs(r.Value, r.Origin)
	case contract.CannotStartNoNodeParsed:
		return NoNodeParsed
	case contract.CannotStartManifestRead:
		return "读取内容时发生错误: " + r.Origin

	// 材料自检：`-s` 那份脚本。
	case contract.CannotStartScriptIsDir:
		return "-s 参数指定的路径是目录，不是脚本文件：" + r.Place + hintTail("请指向一个脚本文件")
	case contract.CannotStartScriptUnreadable:
		return "-s 参数指定的脚本文件读不到：" + r.Place + reasonTail(r.Origin) +
			hintTail(unreadableHint)
	case contract.CannotStartScriptEmpty:
		return "-s 参数指定的脚本文件为空：" + r.Place + hintTail("请先写入内容再执行")
	case contract.CannotStartScriptBinary:
		return "-s 参数指定的脚本是二进制文件：" + r.Place + hintTail("请换一个文本格式的脚本")
	case contract.CannotStartScriptNotUTF8:
		return "-s 参数指定的脚本不是 UTF-8 编码：" + r.Place + hintTail("请把脚本转成 UTF-8 后再执行")

	// 材料自检：下载模式那个本机目录。
	case contract.CannotStartDownloadPathUnreadable:
		return "下载模式：-p 参数指定的本地目录读不到：" + r.Place + reasonTail(r.Origin) +
			hintTail(downloadDirHint)
	case contract.CannotStartDownloadPathNotDir:
		return "下载模式：-p 参数指定的本地路径不是目录：" + r.Place + hintTail(downloadDirHint)

	// 材料自检与读清单共用的 `-f` 三句。
	case contract.CannotStartCSVUnreadable:
		return "-f 参数指定的 CSV 文件读不到：" + r.Place + reasonTail(r.Origin) + hintTail(unreadableHint)
	case contract.CannotStartCSVEmpty:
		return "-f 参数指定的 CSV 文件为空：" + r.Place + hintTail(csvEmptyHint)
	case contract.CannotStartCSVBinary:
		return "-f 参数指定的清单是二进制文件：" + r.Place + hintTail("清单要是文本文件（CSV）")

	// 读清单这一步：内联值带换行、那不是文件、CSV 语法错。
	case contract.CannotStartManifestInlineNewline:
		return "-f 的内联值里不能有换行：内联只写一条" + hintTail("把节点写进 CSV 文件，再用 -f 指向它")
	case contract.CannotStartManifestPathMissing:
		return "-f 参数指定的文件读不到：" + r.Place + reasonTail("找不到这个文件") +
			hintTail(ManifestGoneHint)
	case contract.CannotStartManifestStatFailed:
		return "-f 参数指定的文件读不到：" + r.Place + reasonTail(r.Origin) + hintTail(ManifestGoneHint)
	case contract.CannotStartManifestSyntaxInline:
		return manifestSyntaxText("内联节点信息的", r.Index)
	case contract.CannotStartManifestSyntaxFile:
		return manifestSyntaxText(r.Place+" 的", r.Index)
	}
	panic("起不来：类别 " + string(r.Kind) + " 在 message.CannotStartText 里没有模板")
}

// reasonTail 是「原因：」那一行：系统原文为空时整行不出现。
func reasonTail(origin string) string {
	if origin == "" {
		return ""
	}
	return reasonLabel + origin
}

// hintTail 是「提示：」那一行：这一处没有提示就整行不出现。
func hintTail(hint string) string {
	if hint == "" {
		return ""
	}
	return hintLabel + hint
}

// originTail 是「出处：」那一行：判据文件里某一条写坏时指到那一项该怎么写。
func originTail(origin string) string {
	if origin == "" {
		return ""
	}
	return "\n出处：" + origin
}

// passwdItem 是改密提示词表条目的前缀：`改密配置第 <序号> 条 <节>：`。
func passwdItem(r contract.CannotStartReason) string {
	return "改密配置第 " + strconv.Itoa(r.Index) + " 条 " + r.Place + "："
}

// dangerItem 是危险规则条目的前缀：`危险命令检测规则第 <序号> 条：`。
func dangerItem(r contract.CannotStartReason) string {
	return "危险命令检测规则第 " + strconv.Itoa(r.Index) + " 条："
}

// 材料自检那几句里重复出现的收尾：两份料（脚本、清单）读不到时去路相同，
// 下载目录那两句也共用一条提示——各自只在这里写一遍。
const (
	unreadableHint  = "请检查文件是否存在、当前用户有没有读权限"
	downloadDirHint = "这个目录要在本机上先建好，工具不会替你建"
	csvEmptyHint    = "一行写一台机器，只写 IP 就能跑（端口、用户名、密码走配置）"
)

// manifestSyntaxText 是 CSV 语法错那一句：`读取节点清单失败：<谁的>第 N 行格式读不了`。
//
// prefix 是「谁的那份」（`内联节点信息的`／`<路径> 的`），两个类别只差这一格。
func manifestSyntaxText(prefix string, line int) string {
	return "读取节点清单失败：" + prefix + "第 " + strconv.Itoa(line) +
		" 行格式读不了——CSV 的双引号要成对出现" + hintTail("值里要用引号时，把里面的引号写成两个")
}
