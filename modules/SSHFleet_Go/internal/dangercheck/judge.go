package dangercheck

import (
	"sort"
	"strings"
)

// 风险级别排序：forbidden 最高（对位旧实现）。
var riskOrder = map[string]int{"forbidden": 0, "high": 1, "medium": 2, "low": 3}

// rm 旗标归一顺序与写法（归一化后固定成这个次序，正则才能写短）。
var flagOrder = []struct{ name, text string }{
	{"recursive", "-r"},
	{"force", "-f"},
	{"no_preserve_root", "--no-preserve-root"},
}

// Match 一处命中。
type Match struct {
	Line      int
	Content   string // 判定用的命令段文本
	RuleName  string
	RiskLevel string
}

// Report 一次检查的结果。
type Report struct {
	Matches    []Match // 按风险降序（forbidden 在前）——多命中全列（spec D34）
	IsScript   bool
	ScriptPath string
}

// Highest 最高风险等级；无命中返回空串。
func (r *Report) Highest() string {
	if r == nil || len(r.Matches) == 0 {
		return ""
	}
	return r.Matches[0].RiskLevel
}

// HasForbidden 是否命中 forbidden。
func (r *Report) HasForbidden() bool { return r.Highest() == "forbidden" }

// Analyze 解析并判定一段命令文本，返回全部命中（纯函数：无打印、无交互、无退出）。
func Analyze(src string, rules *Rules) []Match {
	segments := ParseSource(src)
	var matches []Match
	for _, seg := range segments {
		normalized := normalizeForMatch(seg.Tokens)
		if normalized == "" {
			continue
		}
		rule, level := rules.match(normalized)
		if rule == "" {
			continue
		}
		matches = append(matches, Match{Line: seg.Line, Content: seg.Text, RuleName: rule, RiskLevel: level})
	}
	sort.SliceStable(matches, func(i, j int) bool {
		return riskOrder[matches[i].RiskLevel] < riskOrder[matches[j].RiskLevel]
	})
	return matches
}

// normalizeForMatch 把命令段的 token 归一成用于正则匹配的字符串：
// rm 的旗标拍平为固定次序、目标路径归一化；其他命令 token 原样以单空格连接。
// 末尾补一个空格，让 `\s` 能作为最后一个 token 的边界。
func normalizeForMatch(tokens []string) string {
	if len(tokens) == 0 {
		return ""
	}
	if tokens[0] != "rm" {
		return strings.Join(tokens, " ") + " "
	}

	flags := map[string]bool{}
	var targets []string
	endOfFlags := false
	for _, tok := range tokens[1:] {
		if !endOfFlags && tok == "--" {
			endOfFlags = true
			continue
		}
		if !endOfFlags && strings.HasPrefix(tok, "-") && len(tok) > 1 {
			for f := range parseRmFlags(tok) {
				flags[f] = true
			}
			continue
		}
		targets = append(targets, normalizePath(tok))
	}

	parts := []string{"rm"}
	for _, f := range flagOrder {
		if flags[f.name] {
			parts = append(parts, f.text)
		}
	}
	parts = append(parts, targets...)
	return strings.Join(parts, " ") + " "
}

// parseRmFlags 解析单个 rm 旗标 token，归一为 recursive / force / no_preserve_root。
func parseRmFlags(token string) map[string]bool {
	flags := map[string]bool{}
	if token == "--no-preserve-root" {
		flags["no_preserve_root"] = true
		return flags
	}
	if strings.HasPrefix(token, "--") {
		switch strings.TrimPrefix(token, "--") {
		case "recursive":
			flags["recursive"] = true
		case "force":
			flags["force"] = true
		}
		return flags
	}
	for _, ch := range token[1:] {
		switch ch {
		case 'r', 'R':
			flags["recursive"] = true
		case 'f':
			flags["force"] = true
		}
	}
	return flags
}

// normalizePath 路径归一化：合并重复斜杠、去尾斜杠、词法解析 . 与 ..（相对路径原样返回）。
func normalizePath(raw string) string {
	for strings.Contains(raw, "//") {
		raw = strings.ReplaceAll(raw, "//", "/")
	}
	if !strings.HasPrefix(raw, "/") {
		return raw
	}
	var parts []string
	for _, seg := range strings.Split(raw, "/") {
		switch seg {
		case "", ".":
			continue
		case "..":
			if len(parts) > 0 {
				parts = parts[:len(parts)-1]
			}
		default:
			parts = append(parts, seg)
		}
	}
	if len(parts) == 0 {
		return "/"
	}
	return "/" + strings.Join(parts, "/")
}
