//go:build !windows && !linux

package console

// Width 本工具只发 Windows 与 Linux 两版；其余平台上问不到终端宽度，一律不折行。
func Width() int { return 0 }
