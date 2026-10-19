package dangercheck

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// 风险级别排序：forbidden 最高（对位旧实现）。
//
// 这张表是**等级合法值的唯一来源**：加载时校验规则取值、命中后按它排先后，两处共用。
// low 是四档之一——出厂配置不预置，但写进规则文件必须正常加载（spec 定案 M）。
// MUST NOT 在别处另开一个「允许 low」的特判：那会变成两处真相。
var riskOrder = map[string]int{"forbidden": 0, "high": 1, "medium": 2, "low": 3}

// Match 一处命中（同一条规则的全部命中合成一条）。
type Match struct {
	Lines     []int  // 该规则命中的全部行号，升序去重
	Content   string // 命中行的原文（去首尾空白）；多处命中取第一处
	RuleName  string
	RiskLevel string
	Example   string // 规则里的示例命令：让人一眼认出大概是什么命令触发的
	Reason    string // 规则里的说明：为什么这条要拦
}

// LinesText 行号列表的文本形态：`3, 7, 12`（日志与警告框同一口径）。
func (m Match) LinesText() string {
	parts := make([]string, 0, len(m.Lines))
	for _, n := range m.Lines {
		parts = append(parts, strconv.Itoa(n))
	}
	return strings.Join(parts, ", ")
}

// Note 一条命中在日志里的简述：级别、规则、行号、内容，带示例与说明。
func (m Match) Note() string {
	return fmt.Sprintf("%s（%s，来源行 %s）：%s；示例：%s；说明：%s",
		m.RuleName, m.RiskLevel, m.LinesText(), m.Content, m.Example, m.Reason)
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

// Analyze 判定一段命令原文，返回全部命中（纯函数：无打印、无交互、无退出）。
//
// 判定对象是**原文**：不论命令写在哪种语言里，都按同一份文本匹配（ADR-0019）。
// 整份文本一次匹配、规则不得跨行；命中位置由偏移反算行号，内容取该行原文。
func Analyze(src string, rules *Rules) []Match {
	if rules == nil || len(rules.items) == 0 {
		return nil
	}
	text := prepare(normalizeSrc(src))
	lineStarts := newLineIndex(text)

	var matches []Match
	for i, rule := range rules.items {
		if !rule.enabled() {
			continue
		}
		lines := rules.hitLines(i, text, lineStarts)
		if len(lines) == 0 {
			continue
		}
		matches = append(matches, Match{
			Lines:     lines,
			Content:   lineText(text, lineStarts, lines[0]),
			RuleName:  rule.Name,
			RiskLevel: rule.RiskLevel,
			Example:   rule.Example,
			Reason:    rule.Reason,
		})
	}
	sort.SliceStable(matches, func(i, j int) bool {
		return riskOrder[matches[i].RiskLevel] < riskOrder[matches[j].RiskLevel]
	})
	return matches
}

// normalizeSrc 与下发侧同一份正文（定案 D）：剥 UTF-8 BOM、去掉整块首尾空白。
// 下发侧由 ssh.ScriptBodyOf 做同一件事——检测侧若各读一份不同的文本，「检测的」与
// 「执行的」就不是同一份东西了。
func normalizeSrc(src string) string {
	return strings.TrimSpace(strings.TrimPrefix(src, "\ufeff"))
}

// lineIndex 每一行起始的字节偏移（0 基下标 0 = 第 1 行），用来把命中偏移反算成行号。
type lineIndex []int

func newLineIndex(text string) lineIndex {
	idx := lineIndex{0}
	for i := 0; i < len(text); i++ {
		if text[i] == '\n' {
			idx = append(idx, i+1)
		}
	}
	return idx
}

// lineOf 偏移 → 行号（该偏移之前的换行数 + 1，1 基）。
func (li lineIndex) lineOf(offset int) int {
	return sort.Search(len(li), func(i int) bool { return li[i] > offset }) + 1
}

// lineText 取某一行原文（去首尾空白）；行号越界返回空串。
func lineText(text string, li lineIndex, line int) string {
	if line < 1 || line > len(li) {
		return ""
	}
	start := li[line-1]
	end := len(text)
	if line < len(li) {
		end = li[line] - 1 // 下一行起点前一个字节，就是这一行的换行符
	}
	return strings.TrimSpace(text[start:end])
}
