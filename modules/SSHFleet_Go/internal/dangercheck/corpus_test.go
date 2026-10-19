package dangercheck

import (
	"encoding/csv"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// 语料回归：testdata/dangerous_cases.tsv 是人写的判据——每行的「期望级别 / 期望规则名」
// 按「这条命令在两百台生产机上执行会怎样」判断，**不从规则文件反推**。
//
// 语料是制表符分隔的 CSV：多行脚本用双引号包住整个单元格，单元格里可以有真实换行。
// 所以必须用带引号处理的 CSV 解析器读，按行切会把一条记录拆成几条。

// corpusPath 语料文件位置（模块根的 testdata/）。
func corpusPath(t *testing.T) string {
	t.Helper()
	return filepath.Join("..", "..", "testdata", "dangerous_cases.tsv")
}

// corpusRow 一行语料。
type corpusRow struct {
	Line     int    // 这一行在文件里的起始行号（引号单元格跨行时以起始行为准）
	Command  string // 命令原文（可以是多行脚本）
	Level    string // 期望级别；none 表示不期望命中
	RuleName string // 期望规则名
	Reason   string // 场景判断：为什么这条危险（或为什么不危险）
	From     string // 翻译自
	Note     string // 备注
}

// readCorpus 读取语料并校验列数与「理由」列。
func readCorpus(t *testing.T) []corpusRow {
	t.Helper()
	f, err := os.Open(corpusPath(t))
	if err != nil {
		t.Fatalf("读取语料失败：%v", err)
	}
	defer func() { _ = f.Close() }()

	r := csv.NewReader(f)
	r.Comma = '\t'
	r.FieldsPerRecord = -1
	r.LazyQuotes = false

	var rows []corpusRow
	header := true
	for {
		rec, err := r.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("语料第 %d 行起解析失败：%v", r.InputOffset(), err)
		}
		// 记录第一列的起始行：引号单元格跨行时，按这条记录的起始行算。
		startLine, _ := r.FieldPos(0)
		if header {
			header = false
			continue
		}
		if len(rec) != 6 {
			t.Fatalf("语料第 %d 行起应有 6 列，实得 %d 列：%q", startLine, len(rec), rec)
		}
		rows = append(rows, corpusRow{
			Line: startLine, Command: rec[0], Level: strings.TrimSpace(rec[1]),
			RuleName: strings.TrimSpace(rec[2]), Reason: strings.TrimSpace(rec[3]),
			From: strings.TrimSpace(rec[4]), Note: strings.TrimSpace(rec[5]),
		})
	}
	return rows
}

// 行号跟踪用不上 csv.Reader.Line 之外的花样时，用这一版更直白：
// 每条记录的起始行 = 上一条记录的结束行 + 1。
var corpusRegexChar = regexp.MustCompile(`\\b|\[0-9\]|\(\?:|\.\*|\^|\$`)

// TestCorpusLoads 语料本身要过得去：列数、理由必填、理由里不许出现正则字符。
func TestCorpusLoads(t *testing.T) {
	rows := readCorpus(t)
	if len(rows) < 100 {
		t.Fatalf("语料只有 %d 条，至少应有 100 条", len(rows))
	}
	levelCount := map[string]int{}
	ruleCount := map[string]int{}
	noneCount := 0
	for _, row := range rows {
		if row.Reason == "" {
			t.Errorf("语料第 %d 行起没有「理由」列", row.Line)
		}
		if m := corpusRegexChar.FindString(row.Reason); m != "" {
			t.Errorf("语料第 %d 行起的「理由」里出现了正则字符 %q——理由写的是场景，不是正则", row.Line, m)
		}
		switch row.Level {
		case "forbidden", "high", "medium", "low":
			if row.RuleName == "" {
				t.Errorf("语料第 %d 行起期望 %s，却没写期望规则名", row.Line, row.Level)
			}
		case "none":
			noneCount++
			if row.RuleName != "" {
				t.Errorf("语料第 %d 行起期望 none，却写了规则名 %q", row.Line, row.RuleName)
			}
		default:
			t.Errorf("语料第 %d 行起的期望级别 %q 不是合法取值", row.Line, row.Level)
		}
		levelCount[row.Level]++
		if row.RuleName != "" {
			ruleCount[row.RuleName]++
		}
	}
	t.Logf("语料共 %d 条：forbidden %d / high %d / medium %d / low %d / none %d",
		len(rows), levelCount["forbidden"], levelCount["high"], levelCount["medium"], levelCount["low"], noneCount)
	names := make([]string, 0, len(ruleCount))
	for name := range ruleCount {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		t.Logf("  规则 %-12s 覆盖 %d 条", name, ruleCount[name])
	}
}

// TestDangerousCorpus 逐条比对：命令原文 → 期望命中的规则与级别。
//
// 不通过时按 spec 定案 I 处理：先分辨是规则错了、期望错了，还是两边都说得通；
// 拿不准的列出来交开发者定，**不许改期望去迁就代码**。
func TestDangerousCorpus(t *testing.T) {
	// 规则文件 17 条规则的判定内容由工单 04–08 逐族改写，在此之前不逐条比对：
	// 这一条从 09 起生效。语料的读取与自检（TestCorpusLoads）已经生效。
	if os.Getenv("SSHFLEET_CORPUS_STRICT") != "1" {
		t.Skip("规则内容由工单 04–08 落定后再逐条比对（设置 SSHFLEET_CORPUS_STRICT=1 可提前跑）")
	}
	rules := loadRulesOrFail(t)
	rows := readCorpus(t)

	total, passed := 0, 0
	for _, row := range rows {
		got := Analyze(row.Command, rules)
		gotLevel, gotName := highestMatch(got)
		total++
		if gotLevel == row.Level && gotName == row.RuleName {
			passed++
			continue
		}
		t.Errorf("语料第 %d 行起不通过：%q\n  期望 %s/%s\n  实际 %s/%s\n  理由：%s",
			row.Line, row.Command, row.Level, row.RuleName, gotLevel, gotName, row.Reason)
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
