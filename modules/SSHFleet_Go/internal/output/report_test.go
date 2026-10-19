package output

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sshfleet/internal/cli"
	"sshfleet/internal/config"
	"sshfleet/internal/result"
	"sshfleet/internal/verdict"
)

// 归档报告（report.txt）里分类次序的回归。接缝是导出函数 WriteReport：
// 断言的是落到文件里的内容，不碰内部实现。
//
// 次序判据（2026-10-19 定）：数量不同按台数排（报告是升序，对位旧报告）；
// **数量相同时按分类首次出现的先后**——两次跑同一批机器，报告要能逐行比对。

func loadReportKeywords(t *testing.T) *verdict.Keywords {
	t.Helper()
	kw, err := verdict.LoadKeywords(filepath.Join("..", "..", "config", "keywords_error.conf"))
	if err != nil {
		t.Fatalf("关键词文件应可加载：%v", err)
	}
	return kw
}

// writeReportTo 写一份报告并回读全文。dir 由 t.TempDir 给，产物不出工作区。
func writeReportTo(t *testing.T, stats *result.Stats) string {
	t.Helper()
	dir := t.TempDir()
	err := WriteReport(dir, stats, &cli.Args{Command: "whoami"}, &config.Config{},
		[]string{"sshfleet", "-c", "whoami"}, loadReportKeywords(t))
	if err != nil {
		t.Fatalf("写报告失败：%v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, config.BuiltinPaths.Report))
	if err != nil {
		t.Fatalf("读报告失败：%v", err)
	}
	return string(data)
}

// reportCategoryOrder 取「节点清单统计」段里分类出现的先后。
// 分类行形如 `连接超时（2）：`，段头之后、成功分类那一行之前。
func reportCategoryOrder(t *testing.T, report string) []string {
	t.Helper()
	lines := strings.Split(report, "\n")
	start := -1
	for i, ln := range lines {
		if strings.Contains(ln, "【节点清单统计】") {
			start = i + 1
			break
		}
	}
	if start == -1 {
		t.Fatalf("报告中找不到「节点清单统计」段：\n%s", report)
	}

	var order []string
	for _, ln := range lines[start:] {
		if ln == "" || !strings.HasSuffix(ln, "：") || !strings.Contains(ln, "（") {
			continue
		}
		// 段末还会出现成功分类那一行（形如 `执行成功（1）：`），它不属于失败分类段——由
		// 调用方给出的期望值里不含它来兜底，这里只负责按出现顺序收集。
		order = append(order, ln[:strings.LastIndex(ln, "（")])
	}
	return order
}

// 报告里同数量的分类按首现序：手写字面量，期望值独立于被测代码。
func TestWriteReportCategoryOrderByFirstAppearance(t *testing.T) {
	stats := &result.Stats{
		NodesTotal: 7, ResultsTotal: 7, Verify: "通过", SuccessCounts: 0, FailCounts: 7,
		// 统计已按「台数降序、同数量首现序」排好——报告侧的输入就是这个次序。
		SortedFailCategories: []result.CategoryCount{
			{Category: "连接超时", Count: 3},
			{Category: "拒绝网络连接", Count: 2},
			{Category: "原因未知", Count: 2},
		},
		CategoryIPMap: map[string][]string{
			"连接超时":   {"10.0.0.1", "10.0.0.3", "10.0.0.4"},
			"拒绝网络连接": {"10.0.0.2", "10.0.0.6"},
			"原因未知":   {"10.0.0.5", "10.0.0.7"},
		},
	}

	report := writeReportTo(t, stats)

	// 手算：报告按台数升序 → 两个 2 台的在前，3 台的在后；同为 2 台时保持首现序，
	// 即「拒绝网络连接」在「原因未知」之前。
	want := []string{"拒绝网络连接", "原因未知", "连接超时"}
	if got := reportCategoryOrder(t, report); !equalStrings(got, want) {
		t.Errorf("报告里分类次序 = %v，期望 %v\n报告全文：\n%s", got, want, report)
	}
}

// 报告里出现的分类集合，与统计给出的失败分类集合一致——钉住报告侧复用统计次序
// 所依赖的那层不变量（两处都已剔除成功分类）。
func TestWriteReportCategorySetMatchesStats(t *testing.T) {
	stats := &result.Stats{
		NodesTotal: 4, ResultsTotal: 4, Verify: "通过", SuccessCounts: 1, FailCounts: 3,
		SuccessCategory:      "执行成功",
		SortedFailCategories: []result.CategoryCount{{Category: "连接超时", Count: 2}, {Category: "原因未知", Count: 1}},
		CategoryIPMap: map[string][]string{
			"连接超时": {"10.0.0.1", "10.0.0.2"},
			"原因未知": {"10.0.0.3"},
		},
		SuccessIPsCount:  1,
		SortedSuccessIPs: []string{"10.0.0.4"},
	}

	report := writeReportTo(t, stats)
	got := reportCategoryOrder(t, report)

	// 清单段末尾还会列出成功分类，它不算失败分类。
	wantCat := map[string]bool{"连接超时": true, "原因未知": true}
	seen := map[string]bool{}
	for _, c := range got {
		if c == "执行成功" {
			continue
		}
		seen[c] = true
	}
	if len(seen) != len(wantCat) {
		t.Fatalf("报告里的失败分类 = %v，期望 %v", got, wantCat)
	}
	for c := range wantCat {
		if !seen[c] {
			t.Errorf("报告里缺少失败分类 %q：%v", c, got)
		}
	}
	if seen["执行成功"] {
		t.Errorf("成功分类不该出现在失败分类段：%v", got)
	}
}

// 报告侧不再自己去遍历分类图取键：同一份统计连写多次，分类次序必须逐字相同。
// （改前这里会随 map 遍历序抖出多种排列。）
func TestWriteReportCategoryOrderStableAcrossWrites(t *testing.T) {
	stats := &result.Stats{
		NodesTotal: 6, ResultsTotal: 6, Verify: "通过", FailCounts: 6,
		SortedFailCategories: []result.CategoryCount{
			{Category: "连接超时", Count: 2},
			{Category: "拒绝网络连接", Count: 2},
			{Category: "原因未知", Count: 2},
		},
		CategoryIPMap: map[string][]string{
			"连接超时":   {"10.0.0.1", "10.0.0.2"},
			"拒绝网络连接": {"10.0.0.3", "10.0.0.4"},
			"原因未知":   {"10.0.0.5", "10.0.0.6"},
		},
	}

	first := reportCategoryOrder(t, writeReportTo(t, stats))
	want := []string{"连接超时", "拒绝网络连接", "原因未知"} // 首现序：三个分类都是 2 台
	if !equalStrings(first, want) {
		t.Fatalf("首次写入的分类次序 = %v，期望 %v（首现序）", first, want)
	}
	for i := 0; i < 20; i++ {
		if got := reportCategoryOrder(t, writeReportTo(t, stats)); !equalStrings(got, first) {
			t.Fatalf("第 %d 次写入的次序变了：%v，首次是 %v", i+2, got, first)
		}
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
