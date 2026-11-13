package criterion

import "strings"

// 引号：把一段值塞进 shell 时，哪些字符要转义、什么时候要包裹。
//
// 三种形状各有语义，不能互相替代（老版 `ssh/command.go` 就是三个函数同住一处）：
//   - EscapeSingle：嵌进既有的一对单引号里，只把 `'` 换掉；
//   - SingleQuoted：整段包成单引号里的文本，交给 `sh -c` 当单个参数；
//   - QuoteForShell：还原成「人能照抄重跑」的命令行写法，安全的不加引号。
//
// 一份定义、四处调用：下发行（`parse`）、远端辅助命令（`execute`）、显示命令（`message`）。
// 前两者下发，末一个只上屏——需求相反（`$` 要不要拦），所以形状分开，规则仍是一条。

// EscapeSingle 把文本里的单引号转义成 `'\”`：闭合、转义一个、重开。
func EscapeSingle(s string) string {
	return strings.ReplaceAll(s, "'", `'\''`)
}

// SingleQuoted 整段包成单引号文本：内部单引号按 `'\”` 转义，
// 保证作为 `sh -c` 的单个参数传递时不发生二次解析。
func SingleQuoted(s string) string {
	return "'" + EscapeSingle(s) + "'"
}

// shellSafeChars 不被 shell 二次解释的字符集：字母数字与 `_-./:=+,@%^`。
// 逗号在内——`-f` 的内联清单是逗号分隔值，引号包裹会让报告里的写法失真。
const shellSafeChars = "_-./:=+,@%^"

// needsQuoting 判断一段参数被 shell 二次拆词／展开的可能性。
// 空白一律要引号（拆词）；`~` `*` `?` `$` 等由默认分支覆盖（展开）。
func needsQuoting(s string) bool {
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case strings.ContainsRune(shellSafeChars, r):
		default:
			return true
		}
	}
	return false
}

// QuoteForShell 把一段**参数原文**还原成等价的命令行写法：需要时单引号包裹，不需要时原样返回。
//
// 用途仅限「打印给人看的命令行」——日志与 report.txt 里的执行命令，不下发、不参与解析。
// 之所以与 SingleQuoted 分开：这里对 `$` 必须补引号（不补，照抄重跑时 `$HOME` 会被展开），
// 而下发那一侧恰恰要交给目标节点展开。
func QuoteForShell(s string) string {
	if s == "" {
		return `''`
	}
	if !needsQuoting(s) {
		return s
	}
	return SingleQuoted(s)
}
