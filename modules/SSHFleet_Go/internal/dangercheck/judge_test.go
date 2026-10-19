package dangercheck

import (
	"reflect"
	"regexp"
	"testing"
)

// 判定层的行为测试：从 Analyze 这个接缝进出（spec「测试决策」）。
// 期望值按「原文里该被看见什么」人写，不从 config/keywords_dangerous.conf 反推。

func testRules(t *testing.T, specs ...Rule) *Rules {
	t.Helper()
	out, err := compileRulesWithRaw(specs, nil)
	if err != nil {
		t.Fatalf("构造测试规则失败: %v", err)
	}
	return out
}

// regexCompile 把规则里的正则单独编出来（测试里需要直接比对正则时用）。
func regexCompile(t *testing.T, p string) *regexp.Regexp {
	t.Helper()
	return regexp.MustCompile(p)
}

func boolPtr(b bool) *bool { return &b }

func linesOf(matches []Match) [][]int {
	out := make([][]int, 0, len(matches))
	for _, m := range matches {
		out = append(out, m.Lines)
	}
	return out
}

// 同一规则命中多处 → 合成一条，行号全列（定案 B）。
func TestAnalyzeMergesHitsOfOneRule(t *testing.T) {
	rules := testRules(t, Rule{Name: "系统重启", Example: "x", Reason: "测试用", Regex: []string{`\breboot\b`}, RiskLevel: "high"})
	got := Analyze("echo 1\nreboot\necho 3\nreboot", rules)
	if len(got) != 1 {
		t.Fatalf("同一规则命中两处应合成一条，实得 %d 条", len(got))
	}
	if want := []int{2, 4}; !reflect.DeepEqual(got[0].Lines, want) {
		t.Fatalf("行号应为 %v，实得 %v", want, got[0].Lines)
	}
	if got[0].Content != "reboot" {
		t.Fatalf("内容应取命中行原文，实得 %q", got[0].Content)
	}
}

// 不同规则各列一块，按风险降序（forbidden 在前）。
func TestAnalyzeListsEachRuleSeparately(t *testing.T) {
	rules := testRules(t,
		Rule{Name: "重启", Example: "x", Reason: "测试用", Regex: []string{`\breboot\b`}, RiskLevel: "high"},
		Rule{Name: "删配置", Example: "x", Reason: "测试用", Regex: []string{`rm -rf /etc`}, RiskLevel: "forbidden"},
	)
	got := Analyze("reboot\nrm -rf /etc", rules)
	if len(got) != 2 {
		t.Fatalf("两条规则各应列一块，实得 %d 块", len(got))
	}
	if got[0].RuleName != "删配置" || got[0].RiskLevel != "forbidden" {
		t.Fatalf("高风险应排在前面，实得首块 %s/%s", got[0].RuleName, got[0].RiskLevel)
	}
}

// 规则里写 (?s) 也不得跨行（定案 B）。
func TestAnalyzeDoesNotMatchAcrossLines(t *testing.T) {
	rules := testRules(t, Rule{Name: "跨行", Example: "x", Reason: "测试用", Regex: []string{`(?s)alpha.*omega`}, RiskLevel: "high"})
	if got := Analyze("alpha\nomega", rules); len(got) != 0 {
		t.Fatalf("不得跨行命中，实得 %v", linesOf(got))
	}
	if got := Analyze("alpha omega", rules); len(got) != 1 {
		t.Fatalf("同一行内应命中，实得 %v", linesOf(got))
	}
}

// 路径片段归一：//x、/etc/、/a/../b 整备成规范形态，且不改变行数（行号不错位）。
func TestAnalyzeNormalizesPathFragmentsKeepLineNumbers(t *testing.T) {
	rules := testRules(t,
		// 行尾哨兵：整备后的文本在每行末尾补一个空格，路径写到行尾也落在词边界上。
		Rule{Name: "删系统目录", Example: "x", Reason: "测试用", Regex: []string{`rm -rf /etc(?:\s|$)`}, RiskLevel: "forbidden"},
		Rule{Name: "删根", Example: "x", Reason: "测试用", Regex: []string{`rm -rf /(?:\s|$)`}, RiskLevel: "forbidden"},
	)
	cases := []struct {
		src  string
		want string
		line int
	}{
		{"echo 1\nrm -rf //etc", "删系统目录", 2},
		{"echo 1\necho 2\nrm -rf /etc/", "删系统目录", 3},
		{"rm -rf /var/../etc/", "删系统目录", 1},
		{"echo 1\n\n\nrm -rf /", "删根", 4},
	}
	for _, c := range cases {
		got := Analyze(c.src, rules)
		if len(got) != 1 {
			t.Fatalf("%q 应命中 1 条，实得 %d 条", c.src, len(got))
		}
		if got[0].RuleName != c.want || got[0].Lines[0] != c.line {
			t.Fatalf("%q 应命中 %s 第 %d 行，实得 %s %v", c.src, c.want, c.line, got[0].RuleName, got[0].Lines)
		}
	}
}

// 匹配用的文本不能在「路径写到行尾」时丢词边界（曾因此在块内补空格）。
func TestPreparedTextKeepsWordBoundaryAfterPath(t *testing.T) {
	text := prepare(normalizeSrc("rm -rf /etc"))
	if !regexCompile(t, `\s`).MatchString(text) {
		t.Fatalf("整备后至少要有可匹配的空白：%q", text)
	}
}

// 同一规则挂多条正则，命中任意一条即该规则命中（定案 A）。
func TestAnalyzeRuleWithSeveralPatterns(t *testing.T) {
	rules := testRules(t, Rule{Name: "递归删", Example: "x", Reason: "测试用", Regex: []string{`\brm -rf `, `\brm -fr `, `\brm --recursive --force `}, RiskLevel: "high"})
	for _, src := range []string{"rm -rf /x", "rm -fr /x", "rm --recursive --force /x"} {
		if got := Analyze(src, rules); len(got) != 1 {
			t.Fatalf("%q 应命中递归删，实得 %v", src, linesOf(got))
		}
	}
	if got := Analyze("rm -f /x", rules); len(got) != 0 {
		t.Fatalf("rm -f 不应命中，实得 %v", linesOf(got))
	}
}

// 判定的对象是原文：同一条 rm -rf /etc 写在六种语言里都命中（01 验收）。
func TestAnalyzeMatchesSameCommandInSixLanguages(t *testing.T) {
	rules := testRules(t, Rule{Name: "删系统目录", Example: "x", Reason: "测试用", Regex: []string{`rm -rf /etc\b`}, RiskLevel: "forbidden"})
	srcs := []string{
		`rm -rf /etc`,
		`python3 -c 'import os; os.system("rm -rf /etc")'`,
		`perl -e 'system("rm -rf /etc")'`,
		`node -e 'require("child_process").execSync("rm -rf /etc")'`,
		`php -r 'shell_exec("rm -rf /etc");'`,
		`lua -e 'os.execute("rm -rf /etc")'`,
	}
	for _, src := range srcs {
		if got := Analyze(src, rules); len(got) != 1 {
			t.Fatalf("%q 应命中，实得 %v", src, linesOf(got))
		}
	}
}
