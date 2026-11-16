package message

// 进度条与传输视图上那几个词：标题、计数、文件条数、耗时。
//
// 原先它们写死在 `execute` 的 `progress.go` 与 `transfer_view.go` 里（`fmt.Sprintf` 带中文），
// 改一句口吻得翻两个包。着色与渲染仍留 `execute`——这里只出字，不出样式。
//
// 文案一个字都没改：这批只是抽构造点。

import (
	"fmt"
	"time"

	"sshfleet2/internal/contract"
)

// ActionProgress 传输进度条的标题：动作名后面接「进度」（「上传进度」「下载进度」）。
//
// 「进度」这两个字只在这里出现，站里不再拿 `ActionName` 自己接。
func ActionProgress(m contract.Mode) string { return ActionName(m) + "进度" }

// TotalProgressLabel 命令模式那条总进度条的标题。
func TotalProgressLabel() string { return "总进度" }

// NodeProgressLabel 传输视图里节点那一条的标题。
func NodeProgressLabel() string { return "节点进度" }

// DoneRatio 「已完成 3/5」：斜杠两边不留空格。
func DoneRatio(done, total int) string { return fmt.Sprintf("已完成 %d/%d", done, total) }

// CountOK 「成功 3」。
func CountOK(n int) string { return fmt.Sprintf("成功 %d", n) }

// CountFail 「失败 1」。
func CountFail(n int) string { return fmt.Sprintf("失败 %d", n) }

// FileCounts 传输视图里每台机器那行的收尾：「共 10 个文件  成功 9  失败 1」——三段之间两个空格。
func FileCounts(total, ok, fail int) string {
	return fmt.Sprintf("共 %d 个文件  成功 %d  失败 %d", total, ok, fail)
}

// Elapsed 耗时写成 `0:00:12`：时分秒，秒四舍五入。进度条与传输视图共用这一份。
func Elapsed(d time.Duration) string {
	total := int(d.Seconds() + 0.5)
	return fmt.Sprintf("%d:%02d:%02d", total/3600, (total%3600)/60, total%60)
}
