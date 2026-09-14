package dangercheck

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// 语料回归（常驻）：旧工程 test/test_dangerous_detection.py 的 81 例 + 规则校验拦截。
// 语料与规则文件都在仓库内（testdata/ 与 config/），换成 mvdan 解析层后识别率不得退化。

func rulesPath(t *testing.T) string {
	t.Helper()
	return filepath.Join("..", "..", "config", "dangerous_keywords.toml")
}

func loadRulesOrFail(t *testing.T) *Rules {
	t.Helper()
	rules, err := LoadRules(rulesPath(t))
	if err != nil {
		t.Fatalf("规则文件应可加载: %v", err)
	}
	return rules
}

func TestDangerousCorpus(t *testing.T) {
	rules := loadRulesOrFail(t)

	f, err := os.Open(filepath.Join("..", "..", "testdata", "dangerous_cases.tsv"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	total, passed := 0, 0
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 3)
		if len(parts) < 2 {
			t.Fatalf("语料行格式异常: %q", line)
		}
		cmd, wantLevel := parts[0], parts[1]
		wantName := ""
		if len(parts) == 3 {
			wantName = parts[2]
		}

		matches := Analyze(cmd, rules)
		gotLevel, gotName := highestMatch(matches)

		total++
		nameOK := wantName == "" || gotName == wantName
		if gotLevel == wantLevel && nameOK {
			passed++
			continue
		}
		t.Errorf("语料不通过: %q\n  期望 %s/%s\n  实际 %s/%s", cmd, wantLevel, wantName, gotLevel, gotName)
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}

	t.Logf("危险检测语料：%d/%d 通过（识别准确率 %.1f%%）", passed, total, float64(passed)/float64(total)*100)
	if passed != total {
		t.Fatalf("识别率退化：%d/%d", passed, total)
	}
}

// highestMatch 取最高风险命中（对位旧语料脚本的 judge）。
func highestMatch(matches []Match) (string, string) {
	if len(matches) == 0 {
		return "none", ""
	}
	best := matches[0] // Analyze 已按风险降序排列
	return best.RiskLevel, best.RuleName
}

func TestRulesValidation(t *testing.T) {
	base, err := LoadRules(rulesPath(t))
	if err != nil {
		t.Fatal(err)
	}

	good := &Rules{items: base.items, patterns: base.patterns}
	if _, err := compileRules(good.items); err != nil {
		t.Fatalf("正常规则文件应通过校验: %v", err)
	}

	cases := []struct {
		name  string
		rules []Rule
	}{
		{"规则清单为空", nil},
		{"规则数量不足", base.items[:3]},
		{"缺少 regex 字段", append(append([]Rule{}, base.items...), Rule{Name: "坏规则", Example: "x", RiskLevel: "high"})},
		{"risk_level 非法", append(append([]Rule{}, base.items...), Rule{Name: "坏规则", Example: "x", RiskLevel: "超危", Regex: "x"})},
		{"正则无法编译", append(append([]Rule{}, base.items...), Rule{Name: "坏规则", Example: "x", RiskLevel: "high", Regex: "([unclosed"})},
	}
	for _, c := range cases {
		if _, err := compileRules(c.rules); err == nil {
			t.Errorf("规则校验应拦截：%s", c.name)
		}
	}
}

func TestUnknownFieldRejected(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "bad.toml")
	content := "[[rules]]\nname='x'\nexample='x'\nregex='x'\nrisk_level='high'\nbogus=1\n"
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadRules(p); err == nil || !strings.Contains(err.Error(), "未识别字段") {
		t.Fatalf("未知字段应报错，实为: %v", err)
	}
}
