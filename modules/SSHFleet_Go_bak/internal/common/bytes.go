// 字节数的人性化显示：全工具单一实现。
//
// 为什么要有这一处：终端进度条、执行日志、交互确认此前各写了一份，
// 而且终端与日志的写法还不一致（「1.5 MB」与「1.50 MB」）——同一个数字
// 会在同一块屏幕上出现两种写法。对位旧 text_utils.format_size。
package common

import "fmt"

// FormatBytes 把字节数转成人类可读形态（1024 进制，保留一位小数）。
// 不足 1 KiB 时按整字节显示。
func FormatBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGTPE"[exp])
}
