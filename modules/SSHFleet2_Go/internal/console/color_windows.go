//go:build windows

package console

import "unsafe"

// colorSupported 报「这次能不能把 ANSI 转义序列写到终端上」。判据照搬老版 detectColor：
//
//  1. 拿不到句柄、或句柄不是控制台（重定向 / 管道）→ 能。这两个场景不在适配范围内，
//     行为与改动前一致：照旧输出转义序列。
//  2. 是控制台、VT 位已开 → 能。
//  3. 是控制台、VT 位未开 → 替它打开；开成能，开不成不能（本次运行全程输出纯文字，
//     否则序列会被当普通文字逐字节画上屏）。
//
// 为什么程序自己开 VT：Win10 1909 的 conhost 出厂默认不开，而程序无条件输出转义序列，
// 序列就会变成屏幕上的「残骸」。见 ADR-0017。
func colorSupported() bool {
	h, _, _ := procGetStdHandle.Call(stdOutputHandle)
	if h == 0 || h == ^uintptr(0) {
		return true
	}
	var mode uint32
	if r, _, _ := procGetConsoleMode.Call(h, uintptr(unsafe.Pointer(&mode))); r == 0 {
		return true // 不是控制台：重定向 / 管道，照旧输出
	}
	if mode&enableVTProcessing != 0 {
		return true
	}
	if r, _, _ := procSetConsoleMode.Call(h, uintptr(mode|enableVTProcessing)); r == 0 {
		return false
	}
	return true
}
