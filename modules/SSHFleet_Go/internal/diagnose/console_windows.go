//go:build windows

package diagnose

import "golang.org/x/sys/windows"

// ConsoleVT Windows 控制台是否处于 VT（虚拟终端）模式。
//
// 这是「终端会不会按控制字符解释这段字节」的前置条件：VT 没开时，转义序列会被
// 当普通文字逐字节画上屏（历史上有过「残骸」问题，见 ADR-0017）；VT 开着时，
// 回车、清行这类控制字符才真正生效。日志里记下它，读的人才能判断终端的行为。
//
// 拿不到控制台句柄（重定向到文件、或不是 Windows 控制台）时返回 false 与说明。
func ConsoleVT() (enabled bool, note string) {
	h, err := windows.GetStdHandle(windows.STD_OUTPUT_HANDLE)
	if err != nil {
		return false, "拿不到控制台句柄"
	}
	var mode uint32
	if err := windows.GetConsoleMode(h, &mode); err != nil {
		return false, "不是控制台（输出被重定向或经管道）"
	}
	if mode&windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING == 0 {
		return false, "控制台未开启 VT"
	}
	return true, "控制台已开启 VT"
}
