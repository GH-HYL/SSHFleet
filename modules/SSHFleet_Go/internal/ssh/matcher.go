package ssh

import (
	"fmt"
	"regexp"
	"strings"
)

// MatchOptions 触发词与中止词的匹配口径（配置 [interactive] 的两个开关）。
type MatchOptions struct {
	Regex         bool // true：关键词当正则；false：按子串包含
	CaseSensitive bool // true：区分大小写；false：不区分
}

// ValidateKeywords 按下达口径预编译关键词：正则模式下语法非法即报错。
//
// 在选项/参数合规检查阶段调用——非法正则在本地就拦下，不留到运行期静默不匹配
// （那时用户只会看到「触发词未命中」，而真原因是触发词自己写错了）。
func ValidateKeywords(keywords []string, opts MatchOptions) error {
	if !opts.Regex {
		return nil
	}
	for _, kw := range keywords {
		if _, err := regexp.Compile(patternOf(kw, opts.CaseSensitive)); err != nil {
			return fmt.Errorf("触发词不是合法正则：%s\n原因：%v", kw, err)
		}
	}
	return nil
}

// patternOf 关键词 → 正则：不区分大小写用内联标记表达，不再对正文做大小写转换
// （转换会改变个别非 ASCII 字符的字节长度，而游标是按字节推进的）。
func patternOf(keyword string, caseSensitive bool) string {
	if caseSensitive {
		return keyword
	}
	return "(?i)" + keyword
}

// keywordRule 一组关键词：一条代填的触发词，或整组中止词。
type keywordRule struct {
	keywords []string
	patterns []*regexp.Regexp // 正则模式下与 keywords 对位；子串模式下为空
}

func newKeywordRule(keywords []string, opts MatchOptions) keywordRule {
	r := keywordRule{keywords: keywords}
	if opts.Regex {
		r.patterns = make([]*regexp.Regexp, 0, len(keywords))
		for _, kw := range keywords {
			r.patterns = append(r.patterns, regexp.MustCompile(patternOf(kw, opts.CaseSensitive)))
		}
	}
	return r
}

// find 在 s 里找最先出现的关键词，返回起点、终点与命中的那一条。
// 一条规则里的多个关键词任一命中即可（一条代填挂多个触发词）。
func (r keywordRule) find(s string, opts MatchOptions) (int, int, string, bool) {
	bestStart, bestEnd, bestKeyword := -1, -1, ""
	for i, kw := range r.keywords {
		start, end := -1, -1
		switch {
		case opts.Regex:
			if idx := r.patterns[i].FindStringIndex(s); idx != nil {
				start, end = idx[0], idx[1]
			}
		default:
			if idx := indexFold(s, kw, opts.CaseSensitive); idx >= 0 {
				start, end = idx, idx+len(kw)
			}
		}
		// 零宽命中（如正则 a*）不推进游标，会原地死循环；它也不可能是提示文本，直接不要
		if start < 0 || end <= start {
			continue
		}
		if bestStart < 0 || start < bestStart {
			bestStart, bestEnd, bestKeyword = start, end, kw
		}
	}
	if bestStart < 0 {
		return 0, 0, "", false
	}
	return bestStart, bestEnd, bestKeyword, true
}

// indexFold 子串包含查找；不区分大小写时只折叠 ASCII 大小写。
//
// 为什么不用 strings.ToLower：个别非 ASCII 字符转换后会变字节长度，而游标按字节推进，
// 长度一变游标就错位。只折叠 ASCII 既够用（提示里的英文大小写差异都在这段），
// 又保证偏移在同一条字节串上成立。
func indexFold(s, keyword string, caseSensitive bool) int {
	if caseSensitive || keyword == "" {
		return strings.Index(s, keyword)
	}
	return strings.Index(lowerASCII(s), lowerASCII(keyword))
}

// lowerASCII 只折叠 ASCII 大写字母；无大写时原样返回（不额外分配）。
func lowerASCII(s string) string {
	var b []byte
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c < 'A' || c > 'Z' {
			continue
		}
		if b == nil {
			b = []byte(s)
		}
		b[i] = c + ('a' - 'A')
	}
	if b == nil {
		return s
	}
	return string(b)
}

// cursorMatcher 游标匹配器：多条规则并行参与，共用一个游标，命中即推进到触发词末尾。
//
// 为什么必须共用游标：读到的字节流可能只含半个提示，游标不动就能等后半截到齐；
// 命中后推进到触发词末尾而不是段末——同一次推来的块里可能连着两条提示。
type cursorMatcher struct {
	rules     []keywordRule
	opts      MatchOptions
	tail      string // 游标之后的字节：匹配范围
	done      []bool // 各规则是否已命中（一条代填只送一次，中止词每节点至多一次）
	remaining int    // 还没命中的规则数；归零即尾部与游标都不再需要
}

// newCursorMatcher 按规则建匹配器：rules 每项是一组关键词。
func newCursorMatcher(rules [][]string, opts MatchOptions) *cursorMatcher {
	m := &cursorMatcher{opts: opts, remaining: len(rules), done: make([]bool, len(rules))}
	for _, keywords := range rules {
		m.rules = append(m.rules, newKeywordRule(keywords, opts))
	}
	return m
}

// match 一次命中：规则序号、命中的关键词原文、命中所在行的原文。
type match struct {
	rule    int
	keyword string
	line    string
	end     int // 命中终点：游标推进到此处
}

// feed 追加新到的字节，返回本次新增的命中（按出现先后）。
// 命中的规则即标记为已用。
func (m *cursorMatcher) feed(chunk string) []match {
	if m.remaining == 0 || len(m.rules) == 0 {
		return nil
	}
	m.tail += chunk

	var hits []match
	for {
		hit, ok := m.next()
		if !ok {
			break
		}
		hits = append(hits, hit)
		m.done[hit.rule] = true
		m.remaining--
		m.tail = m.tail[hit.end:] // 游标推进到触发词末尾
	}
	if m.remaining == 0 {
		m.tail = "" // 规则用尽：尾部不再需要（输出本身已由采集缓冲完整留存）
	}
	return hits
}

// next 在游标之后找最先出现的命中：多条规则并行，取位置最靠前者。
// 同一起点上并列时取先定义的规则（遍历顺序即优先级）。
func (m *cursorMatcher) next() (match, bool) {
	best := match{rule: -1, line: ""}
	bestStart := -1
	for i, rule := range m.rules {
		if m.done[i] {
			continue
		}
		start, end, keyword, ok := rule.find(m.tail, m.opts)
		if !ok || (bestStart >= 0 && start >= bestStart) {
			continue
		}
		bestStart = start
		best = match{rule: i, keyword: keyword, line: lineAt(m.tail, start, end), end: end}
	}
	if bestStart < 0 {
		return match{}, false
	}
	return best, true
}

// pending 还没命中的规则序号（1 起，按给定顺序）——收尾「触发词未命中」文案要用。
func (m *cursorMatcher) pending() []int {
	var out []int
	for i, done := range m.done {
		if !done {
			out = append(out, i+1)
		}
	}
	return out
}

// lineAt 命中所在行的原文：该行行首到行尾，不含换行与行尾 \r。
//
// 行首靠游标之后的那份字节找：尾部只从上一次命中之后开始，所以这是「命中所在的那一段」，
// 对中止词足够——它是独立匹配器，首次命中前游标一直停在输出开头。
func lineAt(s string, start, end int) string {
	from := strings.LastIndexByte(s[:start], '\n') + 1
	to := len(s)
	if idx := strings.IndexByte(s[end:], '\n'); idx >= 0 {
		to = end + idx
	}
	return strings.TrimSuffix(s[from:to], "\r")
}
