package message

import (
	"strconv"
	"strings"

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
// 纸上只有类别这一条路：`Body` 那一格已经在 03 号单末尾删掉，谁也没法再往纸上装整句。
func CannotStartText(c contract.CannotStart) string {
	r := c.Reason
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
	case contract.CannotStartKeywordCatDuplicate:
		return "错误分类关键词第 " + strconv.Itoa(r.Index) + " 条与第 " + strconv.Itoa(r.Limit) +
			" 条重名：" + r.Value
	case contract.CannotStartPasswdCatDuplicate:
		return "改密配置第 " + strconv.Itoa(r.Index) + " 条 " + r.Place + " 与第 " +
			strconv.Itoa(r.Limit) + " 条重名：" + r.Value

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

	// 上传源采集：`-u` 那棵树在确认屏之前清点一遍时撞上的毛病。
	case contract.CannotStartUploadRootMissing:
		return "-u 参数指定的上传文件或目录读不到：" + r.Place + reasonTail(r.Origin) +
			hintTail("-u 后面要写本机上真实存在的文件或目录")
	case contract.CannotStartUploadRootIsLink:
		return "没有可上传的文件：-u 指定的路径本身是软链接/快捷方式：" + r.Place
	case contract.CannotStartUploadSpecialType:
		return "不支持的文件类型: " + r.Place + " (FIFO/device/socket)"
	case contract.CannotStartUploadFileUnreadable:
		return "文件不可读: " + r.Place + ": " + r.Origin
	case contract.CannotStartUploadDirUnreadable:
		return "-u 参数指定的上传目录读不了：" + r.Place + reasonTail(r.Origin) +
			hintTail("请检查目录的读权限")
	case contract.CannotStartUploadNothingLeft:
		return "没有可上传的文件：" + r.Place + " 下的文件全部是软链接/快捷方式（已过滤 " +
			strconv.Itoa(r.Index) + " 个：" + namesPreview(r.Value) + "）"

	// 逐行报告：一行一处错的两屏（03b 号单）。
	case contract.CannotStartNodeRows:
		return nodeRowsText(r)
	case contract.CannotStartCredRows:
		return credRowsText(r)

	// 提交前那几处取值不对（03 号单最后一批）：模板仍住各自的文件，这里只转派。
	case contract.CannotStartAnswerFileUnreadable:
		return AnswerFileUnreadable(r.Place, r.Origin)
	case contract.CannotStartAnswerFileBinary:
		return AnswerFileIsBinary(r.Place)
	case contract.CannotStartAnswerInlineNewline:
		return AnswerInlineHasNewline()
	case contract.CannotStartAnswerGone:
		return AnswerGone(r.Place, r.Origin)
	case contract.CannotStartAnswerSyntax:
		return AnswerSyntaxError(r.Place, r.Index)
	case contract.CannotStartAnswerRowInvalid:
		return AnswerRowInvalid(r.Place, r.Index, r.Value, r.Origin)
	case contract.CannotStartAnswerEmpty:
		return AnswerEmpty(r.Place)
	case contract.CannotStartAnswerMissingTrigger:
		return AnswerMissingTrigger(r.Index, r.Value)
	case contract.CannotStartAnswerBadRegex:
		return AnswerBadRegex(r.Index, r.Value, r.Origin)

	case contract.CannotStartPasswdCipherEncrypted:
		return PasswdValueIsCipherEncrypted()
	case contract.CannotStartPasswdCipherPlain:
		return PasswdValueIsCipherPlain()
	case contract.CannotStartPasswdUnreadable:
		return PasswdValueUnreadable(r.Place, r.Origin)
	case contract.CannotStartPasswdUndecodable:
		return PasswdValueUndecodable(r.Origin)
	case contract.CannotStartPasswdProblems:
		return PasswdValueProblems(r.Value)
	case contract.CannotStartPasswdNewline:
		return PasswdHasNewline()
	case contract.CannotStartPasswdOuterSpace:
		return PasswdHasOuterSpace()

	case contract.CannotStartKeyToolAlone:
		return KeyToolNeedAlone(r.Value)
	case contract.CannotStartKeyToolWithBatch:
		return KeyToolWithBatch(r.Place, r.Value)

	// 密钥管理那十三屏（03 号单）：模板仍住 keytool.go，这里只转派。
	case contract.CannotStartKeyNoMasterKey:
		return KeyNoMasterKey(r.Key)
	case contract.CannotStartKeyConvertBlocked:
		return KeyConvertBlocked(r.Key)
	case contract.CannotStartKeyConfigUnreadable:
		return KeyConfigUnreadable(r.Origin)
	case contract.CannotStartKeyGenNoOverwrite:
		return KeyGenNoOverwrite()
	case contract.CannotStartKeyGenSaveFailed:
		return KeyGenSaveFailed(r.Key, r.Place, r.Origin)
	case contract.CannotStartKeyConvertReadProblem:
		return KeyConvertReadProblem(contract.CredFileProblem(r.Value), r.Place, r.Origin)
	case contract.CannotStartKeyConvertDecryptFail:
		return KeyConvertDecryptFailed(r.Place, r.Origin)
	case contract.CannotStartKeyConvertEmpty:
		return KeyConvertEmpty(r.Place)
	case contract.CannotStartKeyConvertWriteFail:
		return KeyConvertWriteFailed(r.Place, r.Origin)
	case contract.CannotStartKeyConvertVerifyFail:
		return KeyConvertVerifyFailed(r.Place, r.Origin)
	// 这两条就是那一句系统原话，没有半句工具自己的话可加。
	case contract.CannotStartKeyGenFailed, contract.CannotStartKeyEncryptFailed:
		return r.Origin

	// 内部故障那两屏（03 号单）。
	case contract.CannotStartRunMetaMarshal:
		return "封装运行上下文：" + r.Origin
	case contract.CannotStartRunMetaCipher:
		return "运行上下文：建不出密码器：" + r.Origin
	case contract.CannotStartRunMetaGCM:
		return "运行上下文：建不出 GCM：" + r.Origin
	case contract.CannotStartOutputTerminal:
		return "建终端输出文件失败（" + r.Place + "）：" + r.Origin
	case contract.CannotStartOutputLog:
		return "建执行期日志失败（" + r.Place + "）：" + r.Origin
	}
	panic("起不来：类别 " + string(r.Kind) + " 在 message.CannotStartText 里没有模板")
}

// nodeRowsText 是逐台补全那一屏：表头一句，下面一行一台。
//
// 一行里的几条原因用「，」连起来——它们说的是同一台机器的几处毛病，不该拆成几行。
func nodeRowsText(r contract.CannotStartReason) string {
	var b strings.Builder
	b.WriteString("发现以下错误:")
	for _, row := range r.Rows {
		b.WriteString("\n行 " + strconv.Itoa(row.Line) + " (IP: " + shownIP(row.IP) + "): - " +
			rowItemsText(row.Items))
	}
	return b.String()
}

// credRowsText 是凭据预检那一屏：先说共几处，下面缩两格一行一处，末尾一句去路。
func credRowsText(r contract.CannotStartReason) string {
	var b strings.Builder
	b.WriteString("CSV凭据预检查失败，共 " + strconv.Itoa(len(r.Rows)) + " 处错误：")
	for _, row := range r.Rows {
		b.WriteString("\n  ")
		if row.Line > 0 {
			b.WriteString("行 " + strconv.Itoa(row.Line) + " (IP: " + row.IP + "): ")
		}
		b.WriteString(rowItemsText(row.Items))
	}
	b.WriteString("\n请修复上述问题后重新执行")
	return b.String()
}

// shownIP 是清单第一列怎么露在屏上：没填时写「空值」，否则原样。
func shownIP(ip string) string {
	if ip == "" {
		return "空值"
	}
	return ip
}

// rowItemsText 把一行里那几条拼起来：它们同属一行，用「，」分段。
func rowItemsText(items []contract.CannotStartReason) string {
	parts := make([]string, 0, len(items))
	for _, it := range items {
		parts = append(parts, rowItemText(it))
	}
	return strings.Join(parts, "，")
}

// rowItemText 是逐行报告里的一条：只成半句，不带行号前缀。
func rowItemText(r contract.CannotStartReason) string {
	switch r.Kind {
	case contract.CannotStartRowIPMissing:
		return "IP必须存在"
	case contract.CannotStartRowIPBad:
		return "IP格式不正确"
	case contract.CannotStartRowPortBad:
		return "端口必须是1-65535之间的整数，当前值为：" + r.Value
	case contract.CannotStartRowUserMissing:
		return "用户名为空：清单第 3 列没写，配置 account.user 也没配"
	case contract.CannotStartRowKeyMissing:
		return "用了 -k，但没有找到可用的私钥——在第 5 列写私钥路径，或把统一私钥配到配置 credential.key"
	case contract.CannotStartRowPasswordMissing:
		return "密码为空：清单第 4 列没写，配置 account.password 也没配"

	// 凭据预检那几条：`<哪一格>文件<怎么不对> → <回显>`，系统原话括在末尾。
	case contract.CannotStartRowCredMissing:
		return r.Place + "文件" + credLabelMissing + credShown(r)
	case contract.CannotStartRowCredBadCipher:
		return r.Place + "文件" + credLabelBadCipher + credShown(r)
	case contract.CannotStartRowCredUnreadable:
		return r.Place + "文件" + credLabelUnreadable + credShown(r)
	case contract.CannotStartRowCredConfigPassword:
		return "配置 account.password：" + r.Origin
	case contract.CannotStartRowCredConfigKeyPassword:
		return "配置 credential.key_password：" + r.Origin
	}
	panic("起不来：逐行报告里的类别 " + string(r.Kind) + " 没有模板")
}

// 凭据预检那三种「文件怎么不对」：下一步各是一件不同的事，所以不合成一句。
const (
	credLabelMissing = "不存在（打开加密时这里填的是凭据文件的绝对路径，不加密时才是密码本身）"
	credLabelBadCipher = "不是有效的密文，或主密钥不匹配（先用 --convert-secret 重新转换）"
	credLabelUnreadable = "无法读取"
)

// credShown 拼「→ 回显」那一截，系统报得出原因时再括上它。
func credShown(r contract.CannotStartReason) string {
	out := " → " + r.Value
	if r.Origin != "" {
		out += " (" + r.Origin + ")"
	}
	return out
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

// namesPreview 把一串名字（`、` 连起来）截成最多五个，超出时带一句「等 N 个」。
//
// 清单可能有几百个——一句提示刷满屏没有意义。名字怎么截是**话**，所以住这里，站里只交
// 原始那一串（`CannotStartReason.Value`）。
func namesPreview(joined string) string {
	const maxShow = 5
	if joined == "" {
		return ""
	}
	names := strings.Split(joined, "、")
	if len(names) <= maxShow {
		return joined
	}
	return strings.Join(names[:maxShow], "、") + " 等 " + strconv.Itoa(len(names)) + " 个"
}

// manifestSyntaxText 是 CSV 语法错那一句：`读取节点清单失败：<谁的>第 N 行格式读不了`。
//
// prefix 是「谁的那份」（`内联节点信息的`／`<路径> 的`），两个类别只差这一格。
func manifestSyntaxText(prefix string, line int) string {
	return "读取节点清单失败：" + prefix + "第 " + strconv.Itoa(line) +
		" 行格式读不了——CSV 的双引号要成对出现" + hintTail("值里要用引号时，把里面的引号写成两个")
}
