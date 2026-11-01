//go:build !windows

package console

// colorSupported 非 Windows 平台恒为真：本议题不改非 Windows 的行为，转义序列照旧输出。
func colorSupported() bool { return true }

// adaptCodepage 非 Windows 上没有码页这回事。
func adaptCodepage() {}

// restoreCodepage 同上，空动作。
func restoreCodepage() {}
