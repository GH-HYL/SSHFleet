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
//
// 分类行形如 `连接超时（2）：`，后面跟该分类的 IP 行，段与段之间空一行。但**分类名
// 本身可能跨行、甚至含空行**——兜底分类名由 banner 与报错原文用 "\n" 拼成（见 verdict
// 的 joinText），原文里带什么换行都原样落进名字。所以既不能按「单行结尾是：」认分类行
// （会截掉名字的前半截），也不能按空行切段（名字里的空行会把一段切成两段）。
//
// 分类行唯一的硬定界是末尾那个 `（N）：`——「（」后跟十进制数字、再跟「）：」收到行尾。
// 本函数从每个块的首行起找到这个结尾，合起来即完整分类名。
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
	// 报告里每个分类块由 report.go 的 `"\n%s（%d）：\n"` 写出：**块前必有一个空行**，
	// 其后是名字的若干行（名字最后一行带 `（N）：`），再是该分类的 IP 行，再空行。
	// 于是「空行之后的第一行」就是名字的第一行，往后找到 `（N）：` 收尾即得完整名字。
	//
	// 名字内部可能含空行（兜底分类名由 verdict.joinText 用 "\n" 拼成），出现空行后
	// 不能就地当成下一个块——必须先把本块读完、跳到它的 IP 区之后再说。
	//
	// 已知界限：名字**首行是空行**时，那一个空行与块分隔空行在文本上无法区分，
	// 还原出的名字会少掉开头的空行。这是报告格式不可逆丢的信息，不是本辅助的疏漏。
	i := start
	for i < len(lines) {
		if lines[i] == "" {
			i++
			continue
		}
		// lines[i] 是某个分类名的第一行；找它的结尾行。
		end := -1
		for k := i; k < len(lines); k++ {
			if isCategoryTail(lines[k]) {
				end = k
				break
			}
		}
		if end < 0 {
			break
		}
		name := strings.Join(lines[i:end+1], "\n")
		order = append(order, name[:strings.LastIndex(name, "（")])
		// 跳到本块的 IP 区之后：结尾行之后的所有非空行都是 IP。
		i = end + 1
		for i < len(lines) && lines[i] != "" {
			i++
		}
	}
	return order
}

// isCategoryTail 这一行是不是分类行的结尾，即 `（N）：` 收到行尾、N 是十进制数字。
func isCategoryTail(ln string) bool {
	if !strings.HasSuffix(ln, "）：") {
		return false
	}
	idx := strings.LastIndex(ln, "（")
	if idx < 0 {
		return false
	}
	return isAllDigits(ln[idx+len("（") : len(ln)-len("）：")])
}

// isAllDigits 非空且全是 0-9。
func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
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

	// 清单段末尾还会列出成功分类，它不算失败分类：整段一次判定，既查「失败分类齐不齐」，
	// 也查「成功分类没混进失败分类段」——后者必须在收集时留痕，不能先跳过再断言。
	var gotFail, gotSuccess []string
	for _, c := range got {
		if c == stats.SuccessCategory {
			gotSuccess = append(gotSuccess, c)
			continue
		}
		gotFail = append(gotFail, c)
	}
	wantCat := []string{"连接超时", "原因未知"}
	if !equalStringSet(gotFail, wantCat) {
		t.Fatalf("报告里的失败分类 = %v，期望 %v", gotFail, wantCat)
	}
	if len(gotSuccess) != 1 {
		t.Errorf("清单段里成功分类应恰好出现 1 次（末尾那段），实际 %d 次：%v", len(gotSuccess), got)
	}
	// 失败分类段在前、成功分类段在最后：成功那行必须是全段最后一条分类行。
	if len(got) > 0 && got[len(got)-1] != stats.SuccessCategory {
		t.Errorf("成功分类应排在清单段最后，实际次序 %v", got)
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

// equalStringSet 比两个字符串切片的元素集合（不看次序、不看重复）。
func equalStringSet(got []string, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	seen := make(map[string]bool, len(want))
	for _, w := range want {
		seen[w] = true
	}
	for _, g := range got {
		if !seen[g] {
			return false
		}
	}
	return true
}

// 分类名可能跨行：兜底分类名由 banner 与报错原文用 "\n" 拼成（verdict.joinText），
// 报告里那一行就成了两行物理文本。这里钉住两件事——报告照样把它写成一段带计数的
// 分类行，解析辅助也认得出完整分类名（不会被截成后半截）。
func TestWriteReportMultilineCategoryName(t *testing.T) {
	const cat = "SSH-2.0-OddServer_9.9\nwombat protocol meltdown zzz"
	stats := &result.Stats{
		NodesTotal: 1, ResultsTotal: 1, Verify: "通过", FailCounts: 1,
		SuccessCategory:      "执行成功",
		SortedFailCategories: []result.CategoryCount{{Category: cat, Count: 1}},
		CategoryIPMap:        map[string][]string{cat: {"10.0.0.1"}},
	}

	report := writeReportTo(t, stats)

	// 报告原文：分类名整段照写，"（1）：" 跟在名字最后一行后面。
	wantLine := cat + "（1）："
	if !strings.Contains(report, wantLine) {
		t.Errorf("报告里没有 %q 这一行\n报告全文：\n%s", wantLine, report)
	}
	// 解析辅助认出的是完整分类名，不是后半截。
	if got := reportCategoryOrder(t, report); !equalStrings(got, []string{cat}) {
		t.Errorf("解析出的分类 = %q，期望 %q", got, []string{cat})
	}
}

// 报告的「新密码」行写的是这次要设的新密码的脱敏形态，与加密开关无关。
//
// 加密模式下 --change-password 收的是密文文件路径，真正的密码经 credential 解出来
// 落在 cli.Args.NewPassword。报告属归档产物，只留脱敏形态（spec 留痕条款、手册
// 「日志、报告、参数屏里一律只留前后各 2 位」）；取旗标原值的话，加密时落盘的是
// 一串路径的脱敏，事后翻归档对不上这次到底改成了什么（2026-10-21 实测）。
//
// 期望值按 MaskSecret 的公开规则手算：前 2 位 + **** + 后 2 位。
func TestWriteReportPasswdRowShowsRealNewPassword(t *testing.T) {
	cases := []struct {
		name string
		args *cli.Args
	}{
		{
			name: "加密模式：旗标值是密文文件路径",
			args: &cli.Args{
				ChangePasswordRaw: "/home/hyl/secret/new_pw.txt",
				NewPassword:       "NewPass#2026",
			},
		},
		{
			name: "明文模式：旗标值就是新密码",
			args: &cli.Args{
				ChangePasswordRaw: "NewPass#2026",
				NewPassword:       "NewPass#2026",
			},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			stats := &result.Stats{NodesTotal: 1, ResultsTotal: 1, Verify: "通过", FailCounts: 1}
			if err := WriteReport(dir, stats, c.args, &config.Config{},
				[]string{"sshfleet", "-f", "nodes.csv", "--change-password", c.args.ChangePasswordRaw},
				loadReportKeywords(t)); err != nil {
				t.Fatalf("写报告失败：%v", err)
			}
			data, err := os.ReadFile(filepath.Join(dir, config.BuiltinPaths.Report))
			if err != nil {
				t.Fatalf("读报告失败：%v", err)
			}
			report := string(data)

			want := "  新密码： Ne****26"
			if !strings.Contains(report, want) {
				t.Fatalf("报告应含 %q\n报告全文：\n%s", want, report)
			}
			// 只验「有脱敏形态」是半个断言：还得验明文没跟着落盘。报告是归档产物，
			// 一旦明文漏进去就跟着打包传走。
			if strings.Contains(report, "NewPass#2026") {
				t.Fatalf("报告里不该出现新密码明文\n报告全文：\n%s", report)
			}
		})
	}
}
