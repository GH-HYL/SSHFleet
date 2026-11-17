package dangercheck

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// 语料回归见 corpus_test.go；本文件管规则文件的加载与校验。

func rulesPath(t *testing.T) string {
	t.Helper()
	return filepath.Join("..", "..", "config", "keywords_dangerous.conf")
}

func loadRulesOrFail(t *testing.T) *Rules {
	t.Helper()
	rules, err := LoadRules(rulesPath(t))
	if err != nil {
		t.Fatalf("规则文件应可加载: %v", err)
	}
	return rules
}

func TestRulesValidation(t *testing.T) {
	base, err := LoadRules(rulesPath(t))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := compileRules(base.items); err != nil {
		t.Fatalf("正常规则文件应通过校验: %v", err)
	}

	withBad := func(r Rule) []Rule {
		return append(append([]Rule{}, base.items...), r)
	}
	cases := []struct {
		name  string
		rules []Rule
		want  string // 报错里必须出现的内容：能定位
	}{
		{"规则清单为空", nil, "规则为空"},

		{"缺少 regex 字段", withBad(Rule{Name: "坏规则", Example: "x", Reason: "x", RiskLevel: "high"}), "没有写 regex"},
		{"risk_level 非法", withBad(Rule{Name: "坏规则", Example: "x", Reason: "x", Regex: []string{"x"}, RiskLevel: "超危"}), "risk_level"},
		{"正则无法编译", withBad(Rule{Name: "坏规则", Example: "x", Reason: "x", Regex: []string{"([unclosed"}, RiskLevel: "high"}), "无法编译"},
	}
	for _, c := range cases {
		_, err := compileRules(c.rules)
		if err == nil {
			t.Errorf("规则校验应拦截：%s", c.name)
			continue
		}
		if !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s 的报错没说清是哪一处（缺「%s」）：%v", c.name, c.want, err)
		}
	}
}

// 加载报错要「能定位、能理解、能处置」（定案 O）：指名是哪条规则，并给出可照抄的正确写法。
func TestRuleLoadErrorsNameTheRuleAndShowTheFix(t *testing.T) {
	extra := func(r string) string { return extraRules(r) }
	cases := []struct {
		name, body, wantRule, wantFix string
	}{
		{
			"regex 写成字符串",
			extra("[[rules]]\nname = '坏写法'\nexample = 'x'\nreason = 'x'\nregex = '\\brm'\nrisk_level = 'high'\n"),
			"regex", "数组",
		},
		{
			"reason 超过 30 个显示列",
			extra("[[rules]]\nname = '说明太长'\nexample = 'x'\nreason = '" + strings.Repeat("删", 16) + "'\nregex = ['x']\nrisk_level = 'high'\n"),
			"规则「说明太长」", "30",
		},
		{
			"example 缺失",
			extra("[[rules]]\nname = '缺示例'\nreason = 'x'\nregex = ['x']\nrisk_level = 'high'\n"),
			"规则「缺示例」", "example",
		},
	}
	for _, c := range cases {
		p := filepath.Join(t.TempDir(), "r.conf")
		if err := os.WriteFile(p, []byte(c.body), 0o600); err != nil {
			t.Fatal(err)
		}
		_, err := LoadRules(p)
		if err == nil {
			t.Fatalf("%s 应报错", c.name)
		}
		for _, want := range []string{c.wantRule, c.wantFix, "出路"} {
			if !strings.Contains(err.Error(), want) {
				t.Fatalf("%s 的报错缺「%s」：%v", c.name, want, err)
			}
		}
	}
}

// low 是合法档位：写进去正常加载、正常参与判定（定案 M）。
func TestLowRiskLevelLoads(t *testing.T) {
	p := filepath.Join(t.TempDir(), "r.conf")
	if err := os.WriteFile(p, []byte(extraRules("[[rules]]\nname = '自己加的'\nexample = 'echo hi'\nreason = '只是想看看'\nregex = ['\\becho hi\\b']\nrisk_level = 'low'\n")), 0o600); err != nil {
		t.Fatal(err)
	}
	rules, err := LoadRules(p)
	if err != nil {
		t.Fatalf("low 档应能正常加载：%v", err)
	}
	got := Analyze("echo hi", rules)
	if len(got) != 1 || got[0].RiskLevel != "low" || got[0].RuleName != "自己加的" {
		t.Fatalf("low 规则应正常判定，实得 %+v", got)
	}
}

// 停用的规则只校验 name（定案 N）：其余字段空着、写坏都能加载，且不参与判定。
func TestDisabledRuleOnlyNeedsName(t *testing.T) {
	p := filepath.Join(t.TempDir(), "r.conf")
	body := extraRules("[[rules]]\nname = '停用的'\nenabled = false\n")
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	rules, err := LoadRules(p)
	if err != nil {
		t.Fatalf("停用的规则不应因为其他字段报错：%v", err)
	}
	if got := Analyze("echo hello", rules); len(got) != 0 {
		t.Fatalf("停用的规则不应参与判定，实得 %+v", got)
	}
}

// extraRules 拼一份至少有一条正常规则的规则文件正文，末尾追加一段。
func extraRules(tail string) string {
	one := "[[rules]]\nname = '递归删除'\nexample = 'rm -rf /opt/app'\nreason = '删掉就找不回来'\nregex = ['\\brm -rf ']\nrisk_level = 'high'\n\n"
	return one + tail
}

func TestUnknownFieldRejected(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "bad.toml")
	content := "[[rules]]\nname='x'\nexample='x'\nregex=['x']\nrisk_level='high'\nbogus=1\n"
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadRules(p); err == nil || !strings.Contains(err.Error(), "未识别字段") {
		t.Fatalf("未知字段应报错，实为: %v", err)
	}
}
