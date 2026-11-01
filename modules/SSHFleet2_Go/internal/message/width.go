package message

import "unicode"

// 终端显示宽度与折行：全工具单一实现（ADR-0032 第四条——量、截、折并进拼句包，字只在一处成形）。
//
// 为什么不能按字符数/字节数算：中文、全角标点、emoji 在终端占 2 列，按字符数算会让
// 帮助的列错位。判定规则对位老实现（common/width.go，判据 EAW = W/F）：东亚宽与全角
// 占 2 列；组合记号、零宽字符与空字符占 0 列；其余 1 列。
//
// 这张区间表由 build/eaw_2026-11-01_2215 的生成器从老工程同版的 golang.org/x/text
// （v0.42.0）逐字符扫出来，与老实现逐字符同结果——新工程零第三方依赖，表自己带。

type widthRange struct {
	lo, hi rune
}

// eastAsianWide 是 EAW 属性为 W 或 F 的字符区间，占 2 列。
var eastAsianWide = []widthRange{
	{0x1100, 0x115F},
	{0x231A, 0x231B},
	{0x2329, 0x232A},
	{0x23E9, 0x23EC},
	{0x23F0, 0x23F0},
	{0x23F3, 0x23F3},
	{0x25FD, 0x25FE},
	{0x2614, 0x2615},
	{0x2648, 0x2653},
	{0x267F, 0x267F},
	{0x2693, 0x2693},
	{0x26A1, 0x26A1},
	{0x26AA, 0x26AB},
	{0x26BD, 0x26BE},
	{0x26C4, 0x26C5},
	{0x26CE, 0x26CE},
	{0x26D4, 0x26D4},
	{0x26EA, 0x26EA},
	{0x26F2, 0x26F3},
	{0x26F5, 0x26F5},
	{0x26FA, 0x26FA},
	{0x26FD, 0x26FD},
	{0x2705, 0x2705},
	{0x270A, 0x270B},
	{0x2728, 0x2728},
	{0x274C, 0x274C},
	{0x274E, 0x274E},
	{0x2753, 0x2755},
	{0x2757, 0x2757},
	{0x2795, 0x2797},
	{0x27B0, 0x27B0},
	{0x27BF, 0x27BF},
	{0x2B1B, 0x2B1C},
	{0x2B50, 0x2B50},
	{0x2B55, 0x2B55},
	{0x2E80, 0x2E99},
	{0x2E9B, 0x2EF3},
	{0x2F00, 0x2FD5},
	{0x2FF0, 0x2FFB},
	{0x3000, 0x303E},
	{0x3041, 0x3096},
	{0x3099, 0x30FF},
	{0x3105, 0x312F},
	{0x3131, 0x318E},
	{0x3190, 0x31E3},
	{0x31F0, 0x321E},
	{0x3220, 0x3247},
	{0x3250, 0x4DBF},
	{0x4E00, 0xA48C},
	{0xA490, 0xA4C6},
	{0xA960, 0xA97C},
	{0xAC00, 0xD7A3},
	{0xF900, 0xFAFF},
	{0xFE10, 0xFE19},
	{0xFE30, 0xFE52},
	{0xFE54, 0xFE66},
	{0xFE68, 0xFE6B},
	{0xFF01, 0xFF60},
	{0xFFE0, 0xFFE6},
	{0x16FE0, 0x16FE4},
	{0x16FF0, 0x16FF1},
	{0x17000, 0x187F7},
	{0x18800, 0x18CD5},
	{0x18D00, 0x18D08},
	{0x1AFF0, 0x1AFF3},
	{0x1AFF5, 0x1AFFB},
	{0x1AFFD, 0x1AFFE},
	{0x1B000, 0x1B122},
	{0x1B132, 0x1B132},
	{0x1B150, 0x1B152},
	{0x1B155, 0x1B155},
	{0x1B164, 0x1B167},
	{0x1B170, 0x1B2FB},
	{0x1F004, 0x1F004},
	{0x1F0CF, 0x1F0CF},
	{0x1F18E, 0x1F18E},
	{0x1F191, 0x1F19A},
	{0x1F200, 0x1F202},
	{0x1F210, 0x1F23B},
	{0x1F240, 0x1F248},
	{0x1F250, 0x1F251},
	{0x1F260, 0x1F265},
	{0x1F300, 0x1F320},
	{0x1F32D, 0x1F335},
	{0x1F337, 0x1F37C},
	{0x1F37E, 0x1F393},
	{0x1F3A0, 0x1F3CA},
	{0x1F3CF, 0x1F3D3},
	{0x1F3E0, 0x1F3F0},
	{0x1F3F4, 0x1F3F4},
	{0x1F3F8, 0x1F43E},
	{0x1F440, 0x1F440},
	{0x1F442, 0x1F4FC},
	{0x1F4FF, 0x1F53D},
	{0x1F54B, 0x1F54E},
	{0x1F550, 0x1F567},
	{0x1F57A, 0x1F57A},
	{0x1F595, 0x1F596},
	{0x1F5A4, 0x1F5A4},
	{0x1F5FB, 0x1F64F},
	{0x1F680, 0x1F6C5},
	{0x1F6CC, 0x1F6CC},
	{0x1F6D0, 0x1F6D2},
	{0x1F6D5, 0x1F6D7},
	{0x1F6DC, 0x1F6DF},
	{0x1F6EB, 0x1F6EC},
	{0x1F6F4, 0x1F6FC},
	{0x1F7E0, 0x1F7EB},
	{0x1F7F0, 0x1F7F0},
	{0x1F90C, 0x1F93A},
	{0x1F93C, 0x1F945},
	{0x1F947, 0x1F9FF},
	{0x1FA70, 0x1FA7C},
	{0x1FA80, 0x1FA88},
	{0x1FA90, 0x1FABD},
	{0x1FABF, 0x1FAC5},
	{0x1FACE, 0x1FADB},
	{0x1FAE0, 0x1FAE8},
	{0x1FAF0, 0x1FAF8},
	{0x20000, 0x3FFFF},
}

// runeWidth 单字符的终端显示宽度。
//
// 零宽字符必须算 0 列——变体选择符（U+FE00–FE0F）、组合记号、零宽连接符这些根本不占
// 终端格子。此前一律按 EAW 判定给 1 列，于是「⚠️」（U+26A0 + U+FE0F）被算成 2 列、
// 而终端只画 1 列（老工程 2026-09-15 的教训）。
func runeWidth(r rune) int {
	if r == 0 || unicode.In(r, unicode.Mn, unicode.Me, unicode.Cf) {
		return 0
	}
	lo, hi := 0, len(eastAsianWide)
	for lo < hi {
		mid := (lo + hi) / 2
		switch {
		case r < eastAsianWide[mid].lo:
			hi = mid
		case r > eastAsianWide[mid].hi:
			lo = mid + 1
		default:
			return 2
		}
	}
	return 1
}

// displayWidth 字符串的终端显示宽度。
func displayWidth(s string) int {
	n := 0
	for _, r := range s {
		n += runeWidth(r)
	}
	return n
}

// trimToWidth 按显示宽度截断到 limit 列，返回能放下的最长前缀。
func trimToWidth(s string, limit int) string {
	w := 0
	for i, r := range s {
		rw := runeWidth(r)
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

// wrapByWidth 按显示宽度折行：优先在空格 / 全角标点后断，行内没有断点时按宽度硬断
// （单个字符比 width 还宽时也保证每行至少推进一个字符，不会死循环）。
// width <= 0 表示不折行。
func wrapByWidth(text string, width int) []string {
	if width <= 0 || displayWidth(text) <= width {
		return []string{text}
	}

	runes := []rune(text)
	var lines []string
	start := 0
	for start < len(runes) {
		w, end, lastBreak := 0, start, -1
		for end < len(runes) {
			rw := runeWidth(runes[end])
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
			lines = append(lines, trimRightSpace(string(runes[start:lastBreak+1])))
			start = lastBreak + 1
		} else {
			lines = append(lines, trimRightSpace(string(runes[start:end])))
			start = end
		}
		for start < len(runes) && (runes[start] == ' ' || runes[start] == '\t') {
			start++
		}
	}
	return lines
}

// trimRightSpace 裁掉行尾的空格与制表符（strings.TrimRight 的窄版，避免逐处引包）。
func trimRightSpace(s string) string {
	for len(s) > 0 && (s[len(s)-1] == ' ' || s[len(s)-1] == '\t') {
		s = s[:len(s)-1]
	}
	return s
}

// DisplayWidth 报一段文本的终端显示宽度。导出给「判据表里 example／reason 不许超宽」
// 那一条校验用——宽度的算法只住这里一处。
func DisplayWidth(s string) int { return displayWidth(s) }
