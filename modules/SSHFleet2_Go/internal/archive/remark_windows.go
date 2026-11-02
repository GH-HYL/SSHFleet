//go:build windows

package archive

import "strings"

// remarkInvalidChars 备注里必须换掉的字符（Windows 侧）。
//
// 九个别名非法：`< > : " / \ | ? *`；`/` 与 `\` 还兼作路径分隔符。这九个在 Linux 上
// 完全合法，所以两侧分开写——能用的直接用、不能用的才换，按运行平台判。
const remarkInvalidChars = "<>:\"/\\|?*"

// remarkReservedNames Windows 保留设备名：整段名字（不含扩展名）等于其中之一就不能用，
// 大小写不敏感。命中就补一个下划线，让系统不再把它当设备。
var remarkReservedNames = []string{
	"con", "prn", "aux", "nul",
	"com0", "com1", "com2", "com3", "com4", "com5", "com6", "com7", "com8", "com9",
	"lpt0", "lpt1", "lpt2", "lpt3", "lpt4", "lpt5", "lpt6", "lpt7", "lpt8", "lpt9",
}

// sanitizeRemark 见 remark_unix.go 的同名函数：丢原字符换占位符，不是转义。
func sanitizeRemark(s string) string {
	out := strings.Map(func(r rune) rune {
		if strings.ContainsRune(remarkInvalidChars, r) {
			return '_'
		}
		return r
	}, s)
	lower := strings.ToLower(out)
	for _, reserved := range remarkReservedNames {
		if lower == reserved {
			return out + "_"
		}
	}
	return out
}
