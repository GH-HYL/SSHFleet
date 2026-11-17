//go:build !windows

package common

// detectColor 非 Windows 平台恒返回「启用」——本议题不改非 Windows 的行为。
func detectColor() bool { return true }
