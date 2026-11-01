//go:build linux

package console

import (
	"os"
	"syscall"
	"unsafe"
)

// tiocgwinsz 是 Linux 取窗口大小的 ioctl 命令号。
const tiocgwinsz = 0x5413

type winsize struct {
	row, col, xpixel, ypixel uint16
}

// Width 报当前终端的列数；不是终端（重定向 / 管道）、调用失败，一律返回 0（不折行）。
func Width() int {
	var ws winsize
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, os.Stdout.Fd(), tiocgwinsz, uintptr(unsafe.Pointer(&ws)))
	if errno != 0 || ws.col == 0 {
		return 0
	}
	return int(ws.col)
}
