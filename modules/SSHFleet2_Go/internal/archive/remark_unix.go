//go:build !windows

package archive

import "strings"

// remarkInvalidChars 备注里必须换掉的字符（POSIX 侧）。
//
// **只有 `/` 一个。** POSIX 下除 NUL 外任何字符都能进文件名，`/` 在这里不是「非法字符」，
// 而是路径分隔符——留着它，`-r a/b` 会被内核理解成「建 a，再在 a 里建 b」，归档目录
// 平白多出一层（老版 2026-10-15 实测）。其余字符（空格、冒号、星号、引号）都合法，
// 换掉它们是丢信息。
const remarkInvalidChars = "/"

// sanitizeRemark 把备注里平台不允许的字符换成下划线。
//
// 叫「替换」不叫「转义」：丢原字符换占位符，不是加标记让原字符安全通过——文件系统层面
// 没有 escape 手段。
func sanitizeRemark(s string) string {
	return strings.Map(func(r rune) rune {
		if strings.ContainsRune(remarkInvalidChars, r) {
			return '_'
		}
		return r
	}, s)
}
