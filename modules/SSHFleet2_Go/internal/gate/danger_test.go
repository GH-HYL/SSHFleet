package gate

import (
	"regexp"
	"testing"

	"sshfleet2/internal/contract"
)

// 危险判定（自定义区）：判的是「同一条命令换个写法还认不认」。
//
// 期望值全部写死——`rm -rf //` 那条的内容取自老版实跑的输出（老版整备后印的是规整形态），
// 不拿被测代码自己算一遍。

// testRules 两条最小规则：一条禁止档（根目录）、一条高风险档（/etc）。
// 正则照 `config/keywords_dangerous.conf` 的同名规则抄，去掉与本案无关的分支。
func testRules(t *testing.T) []contract.DangerRule {
	t.Helper()
	compile := func(patterns ...string) []*regexp.Regexp {
		var out []*regexp.Regexp
		for _, p := range patterns {
			re, err := regexp.Compile(p)
			if err != nil {
				t.Fatalf("测试自身的正则写错了：%v", err)
			}
			out = append(out, re)
		}
		return out
	}
	return []contract.DangerRule{
		{
			Name:     "递归删除根目录",
			Level:    contract.RiskForbidden,
			Enabled:  true,
			Patterns: compile(`\brm\s+-rf\s+(?:--no-preserve-root\s+)*/[*.]*(?:[\s;&|)]|$)`),
		},
		{
			Name:     "递归删除系统目录",
			Level:    contract.RiskHigh,
			Enabled:  true,
			Patterns: compile(`\brm\s+-rf\s+/etc(?:/\*)?(?:[\s;&|)]|$)`),
		},
	}
}

func TestAnalyzeNormalizesPaths(t *testing.T) {
	rules := testRules(t)

	cases := []struct {
		name   string
		source string
		rule   string // 期望命中的规则名；空表示应当零命中
		line   int    // 期望首处命中的行号
		text   string // 期望印出来的内容（老版印的是整备后的那一行）
	}{
		{"重复斜杠", "rm -rf //", "递归删除根目录", 1, "rm -rf /"},
		{"尾斜杠", "rm -rf /etc/", "递归删除系统目录", 1, "rm -rf /etc"},
		{"上跳一级", "rm -rf /tmp/../etc", "递归删除系统目录", 1, "rm -rf /etc"},
		{"原样也要命中", "rm -rf /", "递归删除根目录", 1, "rm -rf /"},
		{"行尾就是结尾", "echo start; rm -rf /etc", "递归删除系统目录", 1, "echo start; rm -rf /etc"},
		{"多行取第一处", "echo a\nrm -rf /etc\n", "递归删除系统目录", 2, "rm -rf /etc"},
		{"只删二级目录不拦", "rm -rf /opt/app", "", 0, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			report := analyze(c.source, rules)
			if c.rule == "" {
				if len(report.Matches) != 0 {
					t.Fatalf("不该命中的却命中：%v", report.Matches)
				}
				return
			}
			if len(report.Matches) == 0 {
				t.Fatalf("应当命中 %q，实际零命中", c.rule)
			}
			got := report.Matches[0]
			if got.RuleName != c.rule {
				t.Fatalf("命中的规则是 %q，想要 %q", got.RuleName, c.rule)
			}
			if len(got.Lines) == 0 || got.Lines[0] != c.line {
				t.Fatalf("行号是 %v，想要首处 %d", got.Lines, c.line)
			}
			if got.Content != c.text {
				t.Fatalf("内容是 %q，想要 %q", got.Content, c.text)
			}
		})
	}
}

// 注释里的危险词不算：整行注释、行尾注释都抹成空格，行数不因此改变。
func TestAnalyzeIgnoresComments(t *testing.T) {
	rules := testRules(t)

	for _, source := range []string{
		"# rm -rf /",
		"echo ok # rm -rf /etc",
		"echo $# ; echo done",
		"curl http://example.com/#rm -rf /",
	} {
		if report := analyze(source, rules); len(report.Matches) != 0 {
			t.Fatalf("%q 不该命中，实际命中 %v", source, report.Matches)
		}
	}

	// 整行注释之后那一行仍然按第 2 行报——注释不许把行号压掉。
	report := analyze("# rm -rf /\nrm -rf /etc", rules)
	if len(report.Matches) != 1 || len(report.Matches[0].Lines) != 1 || report.Matches[0].Lines[0] != 2 {
		t.Fatalf("行号应当是 [2]，实际 %v", report.Matches)
	}
}

// 多命中按风险降序：禁止档排在前面，与规则在表里的先后无关。
func TestAnalyzeSortsByRisk(t *testing.T) {
	rules := testRules(t)
	// 把禁止档那条放到后面，确认排序不是照抄输入顺序。
	rules[0], rules[1] = rules[1], rules[0]

	report := analyze("rm -rf /etc\nrm -rf /", rules)
	if len(report.Matches) != 2 {
		t.Fatalf("应当两条都命中，实际 %v", report.Matches)
	}
	if report.Matches[0].Level != contract.RiskForbidden {
		t.Fatalf("首条应当是禁止档，实际 %q", report.Matches[0].Level)
	}
	if report.Matches[1].Level != contract.RiskHigh {
		t.Fatalf("第二条应当是高风险档，实际 %q", report.Matches[1].Level)
	}
}
