//go:build linux

package console

import (
	"os"
	"syscall"
	"unsafe"
)

// tcgets 是 Linux 上取终端属性的 ioctl 命令号。
const tcgets = 0x5401

// termios 只占位：本函数只关心 ioctl 成不成，不读里面的字段。
type termios struct{ data [64]byte }

// isTerminal 标准输出这会儿是不是连在终端上。
//
// 判据与老版的 isatty 同一件事：对标准输出做一次 TCGETS，成功就是终端。**不看列数**——
// 伪终端（`script`、某些 CI、ssh 里的会话）列数可能报 0，但它仍然是终端，界面该出。
func isTerminal() bool {
	var t termios
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, os.Stdout.Fd(), tcgets, uintptr(unsafe.Pointer(&t)))
	return errno == 0
}
