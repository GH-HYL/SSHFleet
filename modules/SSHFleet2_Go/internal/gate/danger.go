package gate

import (
	"regexp"
	"sort"
	"strings"

	"sshfleet2/internal/contract"
)

// 危险命令的判定：按规则判命令原文的危险等级。
//
// 判定对象是**原文**（ADR-0019）：不管命令写在 bash、python、perl、node、php 里，都按同一份
// 文本匹配。判定前做两件整备——去注释、路径片段归一——这两件都只是让同一条命令换个写法也
// 能认出来，不改判定口径。整备口径逐条照老版 `dangercheck/text.go`：只动字面、不建语法结构，
// 输出与输入行数相同（行号是给使用者的坐标）。
//
// 为什么要那一层：`rm -rf //`、`rm -rf /etc/`、`rm -rf /tmp/../etc` 三条指的同一个目录，
// 规则写的是 `/etc` 这种规整形态；不归一就漏报。曾试过「把路径收成文件名」的近似，那会
// 把 `/tmp/../etc` 收成 `etc`，反而什么都对不上。

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
	text := prepare(source)
	lines := strings.Split(text, "\n")

	var matches []contract.DangerMatch
	for _, rule := range rules {
		if !rule.Enabled || len(rule.Patterns) == 0 {
			continue
		}
		var hitLines []int
		content := ""
		for i, line := range lines {
			// 行尾补一个换行再匹配，等价于「行尾也算词边界」——规则常以 \s 收尾，
			// 不补的话最后一行永远匹配不上（老版同法）。
			if !matchesAny(rule.Patterns, line+"\n") {
				continue
			}
			hitLines = append(hitLines, i+1)
			if content == "" {
				content = strings.TrimSpace(line)
			}
		}
		if len(hitLines) == 0 {
			continue
		}
		matches = append(matches, contract.DangerMatch{
			Lines:    dedupe(hitLines),
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

// matchesAny 一条规则的任一正则命中即算命中。
func matchesAny(patterns []*regexp.Regexp, seg string) bool {
	for _, re := range patterns {
		if re.MatchString(seg) {
			return true
		}
	}
	return false
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

// prepare 把原文整备成用于匹配的文本（老版 `prepare`）：去注释 → 路径片段归一。
// 两步都保持行数与换行位置不变。
func prepare(src string) string {
	return normalizePathFragments(stripComments(strings.ReplaceAll(src, "\r\n", "\n")))
}

// stripComments 整行注释、行尾注释不参与匹配：把注释换成空格而不是删行或删列，
// 行数不变，行内其余内容的列位置也不变。
//
// 下面三类 `#` 不是注释起点，必须保留：
//   - 引号里的 `#`（`echo "issue #123"` 里的 `#` 是内容）
//   - `$#`（位置参数个数）
//   - 紧贴字母或数字的 `#`（`http://x/#a` 里的路径片段）
func stripComments(src string) string {
	lines := strings.Split(src, "\n")
	for i, ln := range lines {
		lines[i] = stripLineComment(ln)
	}
	return strings.Join(lines, "\n")
}

// stripLineComment 抹掉一行的注释部分（含整行注释），用空格占位。
func stripLineComment(line string) string {
	cut := -1 // 注释起点；-1 表示整行没有注释
	var quote rune
	prev := rune(0)
	for i, r := range line {
		switch {
		case quote != 0:
			if r == quote {
				quote = 0
			}
		case r == '\'' || r == '"':
			quote = r
		case r == '#' && prev != '$' && !isWordRune(prev):
			// 前面是空白、行首或标点时 `#` 才是注释起点；紧贴字母/数字的是路径片段。
			cut = i
		}
		if cut >= 0 {
			break
		}
		prev = r
	}
	if cut < 0 {
		return line
	}
	return line[:cut] + strings.Repeat(" ", len(line)-cut)
}

// isWordRune 紧贴在 `#` 前面时说明它是内容的一部分（字母、数字、下划线）。
func isWordRune(r rune) bool {
	return r == '_' ||
		(r >= 'a' && r <= 'z') ||
		(r >= 'A' && r <= 'Z') ||
		(r >= '0' && r <= '9') ||
		r > 127 // 非 ASCII 字符一律当内容，宁可少剔也不误剔
}

// pathRuns 在一行里找形如 `/a/b` 的路径片段，返回每段的起止字节位置。
func pathRuns(line string) [][2]int {
	var runs [][2]int
	i := 0
	for i < len(line) {
		if line[i] != '/' {
			i++
			continue
		}
		start := i
		j := i + 1
		for j < len(line) && isPathByte(line[j]) {
			j++
		}
		runs = append(runs, [2]int{start, j})
		i = j
	}
	return runs
}

// isPathByte 路径片段里允许出现的字节。
func isPathByte(b byte) bool {
	switch {
	case b >= 'a' && b <= 'z', b >= 'A' && b <= 'Z', b >= '0' && b <= '9':
		return true
	}
	switch b {
	case '/', '.', '-', '_', '*', '?', '~', '@', '+', '%', '=', ':':
		return true
	}
	return false
}

// normalizePathFragments 把文本里的路径片段按字面整备——合并重复斜杠、去尾斜杠、
// 词法解掉 `.` 与 `..`。顺手在每行末尾补一个空格：规则常以 `\s` 收尾（「路径后面是一个
// 词边界」），不补的话「路径恰好写在行尾」永远命中不了。只补空格不补换行，行数不变。
func normalizePathFragments(src string) string {
	var b strings.Builder
	b.Grow(len(src))
	for _, line := range strings.SplitAfter(src, "\n") {
		eol := ""
		body := line
		if strings.HasSuffix(body, "\n") {
			eol = "\n"
			body = body[:len(body)-1]
		}
		b.WriteString(normalizeLinePaths(body))
		if eol != "" {
			b.WriteString(" ")
		}
		b.WriteString(eol)
	}
	return b.String()
}

// normalizeLinePaths 对一行里的每个路径片段做整备。
func normalizeLinePaths(line string) string {
	runs := pathRuns(line)
	if len(runs) == 0 {
		return line
	}
	var b strings.Builder
	last := 0
	for _, r := range runs {
		b.WriteString(line[last:r[0]])
		b.WriteString(cleanPath(line[r[0]:r[1]]))
		last = r[1]
	}
	b.WriteString(line[last:])
	return b.String()
}

// cleanPath 单个路径片段的词法整备。`*` / `?` 这类通配符按普通字符对待，原样保留。
func cleanPath(p string) string {
	for strings.Contains(p, "//") {
		p = strings.ReplaceAll(p, "//", "/")
	}
	parts := make([]string, 0, 4)
	for _, seg := range strings.Split(p[1:], "/") {
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
