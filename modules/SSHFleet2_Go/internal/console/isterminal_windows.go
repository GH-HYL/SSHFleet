//go:build windows

package console

import "syscall"

// isTerminal 标准输出这会儿是不是连在控制台上。
//
// 判据与老版的 isatty 同一件事：GetConsoleMode 成功就是控制台（重定向到文件、接管道时失败）。
func isTerminal() bool {
	var mode uint32
	if err := syscall.GetConsoleMode(syscall.Stdout, &mode); err != nil {
		return false
	}
	return true
}
