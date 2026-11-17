//go:build windows

package cli

import "strings"

// remarkInvalidChars 备注里必须换掉的字符（Windows）。
//
// 依据 Windows 文件名规则（见 Microsoft「Naming a File」）：
// `< > : " / \ | ? *` 九个字符不能出现在文件名里，另有保留设备名 CON / PRN / AUX / NUL /
// COM0-9 / LPT0-9（大小写不敏感）。
//
// `/` 与 `\` 在 Windows 上还兼作路径分隔符；`/` 在 POSIX 侧也列了，两边都要处理。
//
// 与 POSIX 侧分开的理由：这九个字符在 Linux 上**完全合法**，在那边换掉是丢信息
// （`a:b` → `a_b` 还原不回来）。「能用的直接用、不能用的才换」按运行平台判。
const remarkInvalidChars = "<>:\"/\\|?*"

// remarkReservedNames Windows 保留设备名：整个名字（不含扩展名）等于其中之一就不能用。
// 大小写不敏感——`con` 与 `CON` 同样被系统拒绝。
var remarkReservedNames = []string{
	"con", "prn", "aux", "nul",
	"com0", "com1", "com2", "com3", "com4", "com5", "com6", "com7", "com8", "com9",
	"lpt0", "lpt1", "lpt2", "lpt3", "lpt4", "lpt5", "lpt6", "lpt7", "lpt8", "lpt9",
}

// SanitizeRemark 把备注里平台不允许的字符换成下划线。
//
// 名字叫「转义」不准确：这里是**替换**（丢掉原字符、换一个占位符），不是 escape
// （加标记让原字符安全通过）。文件系统层面没有 escape 手段——`a\:b` 在 Windows 上
// 同样非法——所以只能替换。
//
// 除字符外还要挡保留设备名：整段名字命中就补一个下划线，让系统不再把它当设备。
//
// 导出是给 output 包用的：**显式 -r 与自动生成都要过这里**。只在自动生成那条路上过滤
// 会漏掉显式 -r——实测 `-r a/b` 会被内核当路径，归档目录平白多出一层（2026-10-15）。
func SanitizeRemark(s string) string {
	s = strings.Map(func(r rune) rune {
		if strings.ContainsRune(remarkInvalidChars, r) {
			return '_'
		}
		return r
	}, s)

	// 保留名判定按「主名」做：`nul.txt` 在 Windows 上同样被当设备，扩展名不豁免。
	base := s
	if i := strings.IndexByte(base, '.'); i >= 0 {
		base = base[:i]
	}
	if _, ok := remarkReservedNameSet[strings.ToLower(strings.TrimSpace(base))]; ok {
		s = "_" + s
	}
	return s
}

// remarkReservedNameSet remarkReservedNames 的查找表。
var remarkReservedNameSet = func() map[string]struct{} {
	m := make(map[string]struct{}, len(remarkReservedNames))
	for _, n := range remarkReservedNames {
		m[n] = struct{}{}
	}
	return m
}()
