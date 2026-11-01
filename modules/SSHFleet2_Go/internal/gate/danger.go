package gate

import (
	"sort"
	"strings"

	"sshfleet2/internal/contract"
)

// 危险命令的判定：按规则判命令原文的危险等级。
//
// 判定对象是**原文**（ADR-0019）：不管命令写在 bash、python、perl、node、php 里，都按同一份
// 文本匹配。判定前做两件整备——去注释、把路径片段收成文件名——这两件都只是让同一条命令
// 换个写法也能认出来，不改判定口径。

// riskOrder 是四档的先后：禁止在最前。**顺序只住这里一处**；取值是契约里的常量，解析那
// 边只校验「在不在四档里」。
var riskOrder = map[string]int{
	contract.RiskForbidden: 0,
	contract.RiskHigh:      1,
	contract.RiskMedium:    2,
	contract.RiskLow:       3,
}

// analyze 判一段原文，返回全部命中（按风险降序）。
func analyze(source string, rules []contract.DangerRule) contract.DangerReport {
	var matches []contract.DangerMatch
	for _, rule := range rules {
		if !rule.Enabled || len(rule.Patterns) == 0 {
			continue
		}
		var lines []int
		content := ""
		for i, raw := range strings.Split(strings.ReplaceAll(source, "\r\n", "\n"), "\n") {
			for _, form := range formsOf(raw) {
				if form == "" {
					continue
				}
				hit := false
				for _, re := range rule.Patterns {
					if re.MatchString(form) {
						hit = true
						break
					}
				}
				if hit {
					lines = append(lines, i+1)
					if content == "" {
						content = strings.TrimSpace(raw)
					}
					break
				}
			}
		}
		if len(lines) == 0 {
			continue
		}
		matches = append(matches, contract.DangerMatch{
			Lines:    dedupe(lines),
			Content:  content,
			RuleName: rule.Name,
			Level:    rule.Level,
			Example:  rule.Example,
			Reason:   rule.Reason,
		})
	}

	sort.SliceStable(matches, func(i, j int) bool {
		return riskOrder[matches[i].Level] < riskOrder[matches[j].Level]
	})
	return contract.DangerReport{Matches: matches}
}

// formsOf 一行原文的两种形态：去掉注释的原样，以及把路径片段收成文件名后的样子。
//
// 为什么要收：`/bin/rm -rf /` 与 `rm -rf /` 是同一条命令，规则写的是后者。
func formsOf(line string) []string {
	stripped := stripLineComment(line)
	trimmed := strings.TrimSpace(stripped)
	if trimmed == "" {
		return []string{""}
	}
	return []string{trimmed, basenameTokens(trimmed)}
}

// stripLineComment 去掉行尾注释：引号外的 # 之后都算注释。
func stripLineComment(line string) string {
	var quote rune
	for i, r := range line {
		switch {
		case quote != 0:
			if r == quote {
				quote = 0
			}
		case r == '\'' || r == '"':
			quote = r
		case r == '#':
			return line[:i]
		}
	}
	return line
}

// basenameTokens 把每个以 / 开头的路径片段收成最后一段：`/bin/rm` → `rm`。
func basenameTokens(line string) string {
	fields := strings.Fields(line)
	for i, f := range fields {
		if strings.HasPrefix(f, "/") {
			if idx := strings.LastIndex(f, "/"); idx >= 0 && idx+1 < len(f) {
				fields[i] = f[idx+1:]
			}
		}
	}
	return strings.Join(fields, " ")
}

// dedupe 行号升序去重。
func dedupe(xs []int) []int {
	out := make([]int, 0, len(xs))
	seen := map[int]bool{}
	for _, x := range xs {
		if seen[x] {
			continue
		}
		seen[x] = true
		out = append(out, x)
	}
	sort.Ints(out)
	return out
}
