//go:build !windows

package common

// AdaptConsoleOutputEncoding 非 Windows 平台什么都不做：终端本来就按 UTF-8 解释字节。
func AdaptConsoleOutputEncoding() func() { return func() {} }
