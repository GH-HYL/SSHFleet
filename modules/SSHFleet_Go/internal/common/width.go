// 终端显示宽度与折行：全工具单一实现。
//
// 为什么不能按字符数/字节数算：中文、全角标点、emoji 在终端占 2 列，
// 按字符数算会让框线错位（见危险命令警告框）、让表格列错位（见帮助信息）。
// 判定规则对位旧 text_utils.display_width：东亚宽/全角（EAW = W/F）占 2 列，其余 1 列。
package common

import (
	"strings"

	"golang.org/x/text/width"
)

// RuneWidth 单字符的终端显示宽度。
func RuneWidth(r rune) int {
	switch width.LookupRune(r).Kind() {
	case width.EastAsianWide, width.EastAsianFullwidth:
		return 2
	}
	return 1
}

// DisplayWidth 字符串的终端显示宽度。
func DisplayWidth(s string) int {
	n := 0
	for _, r := range s {
		n += RuneWidth(r)
	}
	return n
}

// TrimToWidth 按显示宽度截断到 limit 列，返回能放下的最长前缀。
func TrimToWidth(s string, limit int) string {
	w := 0
	for i, r := range s {
		rw := RuneWidth(r)
		if w+rw > limit {
			return s[:i]
		}
		w += rw
	}
	return s
}

// isBreakAfter 这些字符之后允许断行（空格与常见全角标点，避免把词从中间劈开）。
func isBreakAfter(r rune) bool {
	switch r {
	case ' ', '\t', '、', '，', '。', '；', '：', '）', '】', '》', ')', ']', '｜', '|':
		return true
	}
	return false
}

// WrapByWidth 按显示宽度折行：优先在空格 / 全角标点后断，行内没有断点时按宽度硬断
// （单个字符比 width 还宽时也保证每行至少推进一个字符，不会死循环）。
// width <= 0 表示不折行。
func WrapByWidth(text string, width int) []string {
	if width <= 0 || DisplayWidth(text) <= width {
		return []string{text}
	}

	runes := []rune(text)
	var lines []string
	start := 0
	for start < len(runes) {
		w, end, lastBreak := 0, start, -1
		for end < len(runes) {
			rw := RuneWidth(runes[end])
			if w+rw > width {
				break
			}
			w += rw
			if isBreakAfter(runes[end]) {
				lastBreak = end
			}
			end++
		}
		if end >= len(runes) { // 剩下的全放得下
			lines = append(lines, string(runes[start:]))
			break
		}
		if end == start { // 单字符超宽：保底切断，避免死循环
			end = start + 1
		}
		// 断点选择：本行已放满，且下一个字符不是可断处（说明会从词中间劈开）时，
		// 才回退到行内最后一个可断点；否则直接断在 end（正好放得下且落在自然边界）。
		if lastBreak > start && !isBreakAfter(runes[end]) {
			lines = append(lines, strings.TrimRight(string(runes[start:lastBreak+1]), " \t"))
			start = lastBreak + 1
		} else {
			lines = append(lines, strings.TrimRight(string(runes[start:end]), " \t"))
			start = end
		}
		for start < len(runes) && (runes[start] == ' ' || runes[start] == '\t') {
			start++
		}
	}
	return lines
}
