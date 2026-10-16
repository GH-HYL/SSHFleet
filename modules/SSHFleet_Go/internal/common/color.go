package common

// ColorEnabled 本次运行能否输出 ANSI 转义序列。
//
// 判定只在**包级初始化**时做一次，此后只读——这保证它早于 main()，更早于任何一行
// 颜色输出（全仓 26 个包级颜色常量都经 ANSI 取它）。平台相关的判定见
// color_windows.go 与 color_other.go。
var ColorEnabled = detectColor()

// ANSI 取一个 ANSI 转义序列：颜色能力不可用时返回空串，调用处那一行自然退成无色文本。
//
// 之所以让颜色常量都经它取值（而不是逐个去改引用点），是因为引用形态全是字符串拼接——
// 值变空串即可完成降级，145 处引用点一处都不用碰。
func ANSI(code string) string {
	if !ColorEnabled {
		return ""
	}
	return code
}
