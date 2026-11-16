package message

import "strconv"

// 那份小 TOML 词法器读不动时的几句（措辞迁移 03 号单）。
//
// 配置与三份判据表共用一个词法器，所以这几句也共用一份字：站里只说「第几行、哪种毛病、
// 牵到哪个名字」，句子在这里成形。这批字原先长在 `parse/toml.go` 的 `fmt.Errorf` 里，
// 屏上经「原因：」那一行原样贴出——迁的时候一个字节都没改。

// TOMLSectionNameEmpty 是「第 N 行的节名是空的」：`[   ]` 这种写法。
func TOMLSectionNameEmpty(line int) string {
	return tomlLine(line) + "的节名是空的"
}

// TOMLNotSectionNorKeyValue 是「第 N 行既不是节也不是键值对：<那一行的原文>」。
//
// 原文照贴：这一处是说不出别的，使用者要照着那一行改。
func TOMLNotSectionNorKeyValue(line int, text string) string {
	return tomlLine(line) + "既不是节也不是键值对：" + text
}

// TOMLKeyNameEmpty 是「第 N 行的键名是空的」：`= 22` 这种写法。
func TOMLKeyNameEmpty(line int) string {
	return tomlLine(line) + "的键名是空的"
}

// TOMLKeyBeforeAnySection 是「第 N 行的键 <键名> 出现在任何节之前」：缺 `[节]` 头。
func TOMLKeyBeforeAnySection(line int, key string) string {
	return tomlLine(line) + "的键 " + key + " 出现在任何节之前"
}

// TOMLDuplicateKey 是「第 N 行的键 <键名> 在 <哪一节> 里写了两次」。
//
// section 是那一节，arrayItem 是它在这一节里的位置：`[[数组表]]` 按「第几条」分作用域，
// 各条目键名重复不叫冲突，所以 0 写普通节 `[x]`，大于 0 写 `[[x]] 第 N 条`。
func TOMLDuplicateKey(line int, key, section string, arrayItem int) string {
	return tomlLine(line) + "的键 " + key + " 在 " + tomlSectionWhere(section, arrayItem) + " 里写了两次"
}

// TOMLParseFailed 是配置读不动那一屏的头一句：「解析 TOML 失败：<原因>」。
//
// 原因就是上面那几句之一，原样接在后面。
func TOMLParseFailed(reason string) string {
	return "解析 TOML 失败：" + reason
}

// tomlLine 是那几句共用的行号前缀：「第 N 行」。
func tomlLine(line int) string {
	return "第 " + strconv.Itoa(line) + " 行"
}

// tomlSectionWhere 是「在哪一节里」的两种写法：普通节 `[x]`，数组表 `[[x]] 第 N 条`。
func tomlSectionWhere(section string, arrayItem int) string {
	if arrayItem == 0 {
		return "[" + section + "]"
	}
	return "[[" + section + "]] 第 " + strconv.Itoa(arrayItem) + " 条"
}
