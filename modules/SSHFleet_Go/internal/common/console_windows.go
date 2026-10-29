//go:build windows

package common

import "golang.org/x/sys/windows"

// cpUTF8 是 UTF-8 的代码页号。x/sys/windows 只包了函数，没包这个常量。
const cpUTF8 uint32 = 65001

// AdaptConsoleOutputEncoding 把控制台的输出码页切成 UTF-8，返回还原函数。
//
// 为什么由程序自己做：我们往 stdout 写的是 UTF-8 字节，而 Windows 出厂控制台是 936
// 码页，同一段字节会被按 GBK 解成 `涓╣慨` 一类——使用者看到的「工具乱码」就是这一环。
// 让他自己去 chcp 或换终端，等于把适配推给他；这与 ADR-0017「控制台适配由程序自己做，
// 不要求使用者配置环境」是同一条口径，颜色能力当初也是这么解决的。决策见 ADR-0024
// 的更新记录（第四节码页那一档改判）。
//
// **只声明、不转码**：字节链路一个字节不动——写文件、写 xlsx、传输都照旧，变的只是
// 「告诉控制台怎么解释这些字节」。所以这不是 ADR-0024 第八节禁止的「码页检测或转码」，
// 而是替使用者把控制台的解释方式摆正。
//
// 三种情形调用方不用管、这里直接不管：拿不到句柄；句柄不是控制台（重定向 / 管道，
// 那时字节直接进文件，码页不参与）；码页本来就是 65001。改不成也不拦——显示不对总比
// 跑不起来强。
//
// 还原是必须的：码页是控制台缓冲区上的全局状态，同一个窗口里别的程序（cmd 的内建命令、
// 别的 CLI）写的是 GBK 字节，我们不还回去，退出之后它们的中文就成了乱码。
func AdaptConsoleOutputEncoding() func() {
	h, err := windows.GetStdHandle(windows.STD_OUTPUT_HANDLE)
	if err != nil {
		return func() {}
	}
	var mode uint32
	// GetStdHandle 在重定向时也会成功返回句柄，「是不是控制台」只能由 GetConsoleMode 判。
	if err := windows.GetConsoleMode(h, &mode); err != nil {
		return func() {}
	}
	orig, err := windows.GetConsoleOutputCP()
	if err != nil || orig == cpUTF8 {
		return func() {}
	}
	if err := windows.SetConsoleOutputCP(cpUTF8); err != nil {
		return func() {}
	}
	restored := false
	return func() {
		// 幂等：正常退出走 defer，致命退出走 exitNow，两条路都会调到这里。
		if restored {
			return
		}
		restored = true
		_ = windows.SetConsoleOutputCP(orig)
	}
}
