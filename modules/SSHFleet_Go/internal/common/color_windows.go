//go:build windows

package common

import "golang.org/x/sys/windows"

// detectColor 探测并适配本机控制台的颜色能力。三步判定，各对应一种真实失败，
// 不做「先试开、失败再判」那样的二次探测：
//
//  1. 拿不到输出句柄、或句柄不是控制台（重定向 / 管道）→ 返回 true。
//     这两个场景不在本议题范围内，行为与改动前一致（照旧输出转义序列）。
//  2. 是控制台、VT 位已开 → 返回 true。
//  3. 是控制台、VT 位未开 → 替它打开 ENABLE_VIRTUAL_TERMINAL_PROCESSING：
//     开成返回 true，开不成返回 false（本次运行全程不输出转义序列，纯文字）。
//
// 为什么要程序自己开：Win10 1909 的 conhost 出厂默认不开 VT，而程序无条件输出转义
// 序列，序列就会被当普通文字逐字节画上屏（客户机的「残骸」）。旧 Python 版靠
// loguru→colorama 在 import 阶段替它开了，Go 版没人做这件事。决策见 ADR-0017。
func detectColor() bool {
	h, err := windows.GetStdHandle(windows.STD_OUTPUT_HANDLE)
	if err != nil {
		return true
	}
	var mode uint32
	// GetStdHandle 在重定向时也会成功返回一个句柄，所以「是不是控制台」只能由
	// GetConsoleMode 判定——它对文件 / 管道句柄会失败。
	if err := windows.GetConsoleMode(h, &mode); err != nil {
		return true
	}
	if mode&windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING != 0 {
		return true
	}
	if err := windows.SetConsoleMode(h, mode|windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING); err != nil {
		return false
	}
	return true
}
