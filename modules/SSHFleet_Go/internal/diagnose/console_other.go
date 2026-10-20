//go:build !windows

package diagnose

// ConsoleVT 非 Windows 平台没有这个概念：终端本身就按控制字符解释。
func ConsoleVT() (enabled bool, note string) {
	return true, "非 Windows 平台，终端本身按控制字符解释"
}
