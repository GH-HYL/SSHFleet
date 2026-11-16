package message

import "strconv"

// 配置写错时那几屏的话：`[env]` 与 `[interpreter]` 两段（措辞迁移 05 号单）。
//
// 每一处都是「哪里错 → 为什么 → 怎么改」三段，或者只有前半句加一个「当前值」。整段在这里
// 成形，`parse` 只递名字与原值；这批字原先长在 `parse/config.go` 的 `fmt.Sprintf` 里，
// 屏上经「原因：」「提示：」两行贴出——迁的时候一个字节都没改。

// quote 是「把使用者写的原值原样括起来」那一种写法：`fmt` 的 `%q` 与它同源（`strconv.Quote`）。
//
// 原值里带引号、反斜杠、换行时，转义由它做——那几屏要让人一眼看出「你写的是这个字符串」。
func quote(s string) string { return strconv.Quote(s) }

// EnvNameInvalid 是 `[env]` 里那个名字 shell 认不出：`export` 只认标识符那一套。
func EnvNameInvalid(name string) string {
	return Problem(
		"env 里的 "+quote(name)+" 不是合法的环境变量名",
		"shell 的 export 只认「字母或下划线开头，后接字母、数字、下划线」的名字，写别的到远端会报 not a valid identifier、变量不生效",
		"改成一个合法名字，例如 MY_VAR")
}

// EnvValueTrailingBackslash 是值以奇数个反斜杠收尾：下发时会被包进双引号，反斜杠吃掉收尾那个引号。
func EnvValueTrailingBackslash(name, value string) string {
	return Problem(
		CurrentValue("env 里的 "+quote(name)+" 取值不能以反斜杠结尾", quote(value)),
		"值会被包进双引号下发，结尾的反斜杠会吃掉那个收尾引号，远端整条命令变成语法错",
		"要一个反斜杠就写两个")
}

// InterpreterCommandEmpty 是 `interpreter.command` 那格空着：命令模式（`-c`）没有解释器可用。
func InterpreterCommandEmpty() string {
	return ProblemHint(
		"interpreter.command 不能为空",
		"写命令模式（-c）要用的解释器，例如 bash")
}

// InterpreterCommandNewline 是 `interpreter.command` 里带换行：一行命令拼进 shell 就成了两行。
func InterpreterCommandNewline(command string) string {
	return CurrentValue("interpreter.command 不能含换行", quote(command))
}

// InterpreterScriptMissing 是 `[interpreter.script]` 一条都没配：脚本模式认不出该用哪个解释器。
func InterpreterScriptMissing() string {
	return ProblemHint(
		"interpreter.script 至少要配一条",
		`写成 ".sh" = "bash" 这样，后缀带点、等号两边都要有值`)
}

// InterpreterSuffixBlank 是某一条的后缀是空串：等号左边没写东西，这条映射没有意义。
func InterpreterSuffixBlank() string {
	return ProblemHint(
		"interpreter.script 里有一条后缀是空的",
		`每条都要写成 ".sh" = "bash"，后缀带点、等号两边都要有值`)
}

// InterpreterSuffixNotDotted 是后缀没带点：清单里的文件按「含点后缀」匹配，写 `sh` 一条都对不上。
func InterpreterSuffixNotDotted(suffix string) string {
	return ProblemHint(
		CurrentValue("interpreter.script 的后缀要以点开头", quote(suffix)),
		`写成 ".sh"，不要写成 "sh"`)
}

// InterpreterValueMissing 是某一条只写了后缀、等号右边空着。
func InterpreterValueMissing(suffix string) string {
	return ProblemHint(
		"interpreter.script 里 "+quote(suffix)+" 没有配解释器",
		`等号右边要写解释器，例如 ".sh" = "bash"`)
}

// InterpreterValueNewline 是某一格的解释器里带换行。
func InterpreterValueNewline(suffix, interpreter string) string {
	return CurrentValue("interpreter.script 里 "+quote(suffix)+" 的解释器不能含换行", quote(interpreter))
}

// InterpreterSuffixDuplicate 是同一个后缀配了两回：`runner` 与 `RUNNER` 在后缀这一列是同一条。
//
// 大小写不区分，所以两个名字都按原样贴出来——改哪一个由使用者定。
func InterpreterSuffixDuplicate(first, second string) string {
	return ProblemHint(
		"interpreter.script 里后缀重复："+quote(first)+" 与 "+quote(second)+" 是同一个后缀（大小写不区分）",
		"一个后缀只能配一个解释器")
}
