//go:build !windows

package cli

import "strings"

// remarkInvalidChars 备注里必须换掉的字符（POSIX）。
//
// **只有 `/` 一个。** 依据 File Name Portability（glibc 手册）：
// 「POSIX 下任何字符（除 NUL）都可用于文件名，且无长度限制」。
// `/` 在这里不是「非法字符」，而是**路径分隔符**——留着它，`-r a/b` 会被内核理解成
// 「建 a，再在 a 里建 b」，归档目录平白多出一层（2026-10-15 实测）。
//
// 其余字符一律原样保留：空格、冒号、星号、问号、引号、竖线在 POSIX 下都是**合法文件名的一部分**，
// 把它们换成下划线是丢信息的（`a:b` 变成 `a_b` 之后还原不回来）。
//
// NUL（\x00）理论上是唯一的另一个禁区，但命令行参数传不进 NUL，故不必列。
var remarkInvalidChars = "/"

// SanitizeRemark 把备注里平台不允许的字符换成下划线。
//
// 名字叫「转义」不准确：这里是**替换**（丢掉原字符、换一个占位符），不是 escape
// （加标记让原字符安全通过）。文件系统层面没有 escape 手段——`a\:b` 在 Windows 上
// 同样非法——所以只能替换。
//
// 导出是给 output 包用的：**显式 -r 与自动生成都要过这里**。只在自动生成那条路上过滤
// 会漏掉显式 -r——实测 `-r a/b` 会被内核当路径，归档目录平白多出一层（2026-10-15）。
func SanitizeRemark(s string) string {
	return strings.Map(func(r rune) rune {
		if strings.ContainsRune(remarkInvalidChars, r) {
			return '_'
		}
		return r
	}, s)
}
