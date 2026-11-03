package message

import "strconv"

// 代填（`-a`）那一路的话：来源判定、读文件、读成行、每行合规、整表合规。
//
// 逐字照老版 `cli/answer.go` 的 checkAnswer* 与 `cli/source.go` 的 checkInlineValue：
// 哪一句是「问题／原因／提示」三段、哪一句只有两段，都照原样——使用者照着做的那一步不一样。

// AnswerFileUnreadable `-a` 给的路径读不到（问题 / 原因 / 提示三段）。
func AnswerFileUnreadable(path, reason string) string {
	return Problem("-a 参数指定的文件读不到："+path, reason,
		"请检查文件是否存在、当前用户有没有读权限")
}

// AnswerFileIsBinary `-a` 指的是一份二进制文件。
func AnswerFileIsBinary(path string) string {
	return ProblemHint("-a 参数指定的代填文件是二进制文件："+path,
		"代填要写成文本（CSV）")
}

// AnswerInlineHasNewline 内联值里有换行：内联只写一条。
func AnswerInlineHasNewline() string {
	return ProblemHint("-a 的内联值里不能有换行：内联只写一条",
		"把代填写进 CSV 文件，再用 -a 指向它；内容里要回车也只能写文件")
}

// AnswerGone 既不是文件、也不像一条代填记录。原因取系统那一句（ADR-0025 决定六）。
func AnswerGone(raw, reason string) string {
	return Problem("-a 参数指定的文件读不到："+raw, reason,
		"直接写代填内容就用逗号分隔：代填内容,触发词,触发词")
}

// AnswerSyntaxError CSV 语法错：说清哪一行、引号要成对。
//
// inline 为真时 where 是空串（内联那一条没有文件名可报）。
func AnswerSyntaxError(where string, line int) string {
	subject := "-a 的值"
	if where != "" {
		subject = "-a 参数指定的代填文件 " + where
	}
	return ProblemHint(
		subject+" 第 "+itoa(line)+" 行格式读不了——CSV 的双引号要成对出现",
		"值里要用引号时，把里面的引号写成两个")
}

// AnswerRowInvalid 某一条代填不合规：行号与那一行的原文都带上。
func AnswerRowInvalid(where string, line int, rowText, why string) string {
	subject := "-a 的值"
	if where != "" {
		subject = "-a 参数指定的代填文件 " + where
	}
	return subject + " 第 " + itoa(line) + " 行不合规：" + rowText + "（" + why + "）"
}

// AnswerRowNoTrigger 这一行缺触发词。
func AnswerRowNoTrigger() string {
	return "这一行缺触发词，至少要两列：第 1 列写代填内容，第 2 列起写触发词"
}

// AnswerRowEmptyTrigger 第 n 列（从 1 数）的触发词是空的。
func AnswerRowEmptyTrigger(col int) string {
	return "第 " + itoa(col) + " 列的触发词是空的：触发词决定这一行在什么提示出现时发出，留空等于无条件盲填"
}

// AnswerEmpty 一份取值里一条代填都没有：报出来，不静默跑空表。
func AnswerEmpty(where string) string {
	subject := "-a 的值"
	if where != "" {
		subject = "-a 参数指定的代填文件 " + where
	}
	return ProblemHint(subject+" 里没有一条代填",
		"一行一条，第 1 列是代填内容、第 2 列起是触发词；空行与 # 开头会被跳过")
}

// AnswerMissingTrigger 整表复查时发现第 n 条缺触发词。
func AnswerMissingTrigger(n int, value string) string {
	return ProblemHint("-a 第 "+itoa(n)+" 条代填缺少触发词："+value,
		"触发词是唯一的门控，没有它等于无条件盲填")
}

// AnswerBadRegex 正则口径下触发词写不成一个合法正则：本地就拦下，不留到运行期静默不匹配。
func AnswerBadRegex(n int, keyword, reason string) string {
	return ProblemReason("-a 第 "+itoa(n)+" 条代填：触发词不是合法正则："+keyword, reason)
}

// itoa 只给上面这几句拼行号用（数字进句子也算成形，所以不放到调用方去）。
func itoa(n int) string {
	return strconv.Itoa(n)
}
