//go:build !windows && !linux

package console

// isTerminal 本工具只发 Windows 与 Linux 两版；其余平台一律按「不是终端」办，走直通。
func isTerminal() bool { return false }
