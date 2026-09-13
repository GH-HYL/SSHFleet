package output

import (
	"fmt"
	"io"

	"sshfleet/internal/batch"
)

// ProgressRenderer 返回进度渲染函数（spec D2 展开：进度渲染归 internal/output，
// 由 main 作为参数注入给 internal/batch）。
//
// M3 只交付可用的最小形态：单行就地刷新的汇总（完成 / 成功 / 失败 + 字节）。
// M5 将替换为完整终端进度条（多节点条形、速度显示、lipgloss 样式）。
func ProgressRenderer(out io.Writer) func(batch.Snapshot) {
	lastLen := 0
	return func(s batch.Snapshot) {
		if s.Total == 0 {
			return
		}
		line := fmt.Sprintf("进度 %d/%d  成功 %d  失败 %d", s.Completed, s.Total, s.Succeeded, s.Failed)
		if s.BytesTotal > 0 {
			line += fmt.Sprintf("  传输 %s/%s", formatSize(s.BytesDone), formatSize(s.BytesTotal))
		}
		pad := ""
		if lastLen > len([]rune(line)) {
			for i := len([]rune(line)); i < lastLen; i++ {
				pad += " "
			}
		}
		lastLen = len([]rune(line))
		fmt.Fprintf(out, "\r%s%s", line, pad)
		if s.Completed == s.Total {
			fmt.Fprintln(out)
		}
	}
}

// formatSize 文件大小人性化（对位旧 text_utils.format_size）。
func formatSize(n int64) string {
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
