//go:build windows

package console

import (
	"syscall"
	"unsafe"
)

// stdOutputHandle 是 STD_OUTPUT_HANDLE（-11）的 uintptr 形态：uintptr 装不下负常数，按补码写。
const stdOutputHandle = ^uintptr(10)

// Width 报当前终端的列数；不是终端（重定向 / 管道）、句柄拿不到、调用失败，一律返回 0。
// 0 的含义是「不折行」——显示得难看总比跑不起来强（老工程同一条口径）。
// 只碰输出端，不读输入、不改模式、不动码页。
func Width() int {
	h, _, _ := procGetStdHandle.Call(stdOutputHandle)
	if h == 0 || h == ^uintptr(0) {
		return 0
	}
	var info consoleScreenBufferInfo
	if r, _, _ := procGetConsoleScreenBufferInfo.Call(h, uintptr(unsafe.Pointer(&info))); r == 0 {
		return 0
	}
	w := int(info.srWindow.right - info.srWindow.left + 1)
	if w <= 0 {
		return 0
	}
	return w
}

// kernel32 走标准库的惰性加载，不引第三方（x/sys 不许用）。
var (
	kernel32                       = syscall.NewLazyDLL("kernel32.dll")
	procGetStdHandle               = kernel32.NewProc("GetStdHandle")
	procGetConsoleScreenBufferInfo = kernel32.NewProc("GetConsoleScreenBufferInfo")
)

type coord struct{ x, y int16 }

type smallRect struct {
	left, top, right, bottom int16
}

// consoleScreenBufferInfo 对位 Win32 的 CONSOLE_SCREEN_BUFFER_INFO，只要 srWindow 那四格。
type consoleScreenBufferInfo struct {
	dwSize              coord
	dwCursorPosition    coord
	wAttributes         uint16
	srWindow            smallRect
	dwMaximumWindowSize coord
}
