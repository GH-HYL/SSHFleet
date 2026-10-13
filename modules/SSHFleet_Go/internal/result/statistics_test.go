package result

import (
	"maps"
	"reflect"
	"slices"
	"testing"
	"time"

	"sshfleet/internal/cli"
	"sshfleet/internal/nodelist"
	"sshfleet/internal/ssh"
	"sshfleet/internal/verdict"
)

// 本文件测两个导出接缝：Statistics 与 SortIPs。
//
// 期望值一律写字面量（手算或规格给的数），不重跑被测算法。
// 分类名两处来源不同，注释里分开标：
//   - 成功分类来自 cli 模式名字表（执行成功 / 传输成功 / 改密成功）；
//   - 失败分类来自判定（internal/verdict），本包只读 r.Category，不改一个字。

func intPtr(v int) *int { return &v }

// argsWith 造一个能定出模式的 Args。只填与 ModeName 相关的字段，
// 其余留零值——统计不读别的参数。
func argsWith(command string) *cli.Args { return &cli.Args{Command: command} }

// nodesOf 造 n 个占位节点。统计只读 nodes.Len()，IP 内容无关。
func nodesOf(n int) *nodelist.Nodes {
	items := make([]nodelist.NodeInfo, n)
	for i := range items {
		items[i] = nodelist.NodeInfo{IP: "127.0.0.1", Port: 22, User: "root"}
	}
	return &nodelist.Nodes{Items: items}
}

// statsOf 跑一次统计，固定时间基准。耗时用整秒，避免浮点比较的精度噪声。
func statsOf(t *testing.T, items []ssh.Result, nodes *nodelist.Nodes, command string, interrupted bool) *Stats {
	t.Helper()
	start := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	stop := start.Add(90 * time.Second)
	return Statistics(items, nodes, argsWith(command), start, stop, interrupted)
}

// —— 接缝一：Statistics ——

// 四个计数口径：成功 / 失败看结论区 Verdict，连接成败看事实区 ConnectSuccess，
// 两者互相独立——连接失败的机器也可能判定为成功（如「密码未过期」走 cleanStop）。
func TestStatisticsCounts(t *testing.T) {
	items := []ssh.Result{
		{IP: "10.0.0.1", ConnectSuccess: true, Verdict: verdict.Success, Category: ""},
		{IP: "10.0.0.2", ConnectSuccess: true, Verdict: verdict.Success, Category: ""},
		// 判定为成功但连接失败：成功计数照样加，连接成功计数不加。
		{IP: "10.0.0.3", ConnectSuccess: false, Verdict: verdict.Success, Category: ""},
		// 连接成功但判定失败：分类取结果里那个。
		{IP: "10.0.0.4", ConnectSuccess: true, Verdict: verdict.Other, Category: "连接超时"},
		{IP: "10.0.0.5", ConnectSuccess: false, Verdict: verdict.Other, Category: "连接超时"},
		{IP: "10.0.0.6", ConnectSuccess: false, Verdict: verdict.Other, Category: "拒绝网络连接"},
	}
	got := statsOf(t, items, nodesOf(6), "whoami", false)

	if got.SuccessCounts != 3 {
		t.Errorf("SuccessCounts = %d, 期望 3", got.SuccessCounts)
	}
	if got.FailCounts != 3 {
		t.Errorf("FailCounts = %d, 期望 3", got.FailCounts)
	}
	if got.ConnectSuccessCounts != 3 {
		t.Errorf("ConnectSuccessCounts = %d, 期望 3", got.ConnectSuccessCounts)
	}
	if got.ConnectFailCounts != 3 {
		t.Errorf("ConnectFailCounts = %d, 期望 3", got.ConnectFailCounts)
	}
	if got.SuccessCounts+got.FailCounts != got.ResultsTotal {
		t.Errorf("成败之和 %d 应等于结果总数 %d", got.SuccessCounts+got.FailCounts, got.ResultsTotal)
	}
}

// 成功分类按模式名字表给：命令 → 执行成功、上传/下载 → 传输成功、改密 → 改密成功。
func TestStatisticsSuccessCategoryByMode(t *testing.T) {
	start := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

	cases := []struct {
		name string
		args *cli.Args
		want string
	}{
		{"命令模式", &cli.Args{Command: "whoami"}, "执行成功"},
		{"脚本模式", &cli.Args{Script: "/tmp/x.sh"}, "执行成功"},
		{"上传模式", &cli.Args{Upload: "/tmp/x"}, "传输成功"},
		{"下载模式", &cli.Args{Download: "/tmp/x"}, "传输成功"},
		{"改密模式", &cli.Args{ChangePassword: "root:newpass"}, "改密成功"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Statistics(nil, nodesOf(0), c.args, start, start, false)
			if got.SuccessCategory != c.want {
				t.Errorf("SuccessCategory = %q, 期望 %q", got.SuccessCategory, c.want)
			}
		})
	}
}

// 成功行的分类一律写成模式给的成功分类，而不是结果自带的 Category；
// 并且成功分类不进失败分类表、也不留在分类 IP 图里。
func TestStatisticsSuccessLinesUseModeCategory(t *testing.T) {
	items := []ssh.Result{
		// 结论区 Category 留空（判定的成功行就是这样），统计要补上成功分类。
		{IP: "10.0.0.9", ConnectSuccess: true, Verdict: verdict.Success},
		{IP: "10.0.0.2", ConnectSuccess: true, Verdict: verdict.Other, Category: "连接超时"},
		{IP: "10.0.0.1", ConnectSuccess: true, Verdict: verdict.Other, Category: "连接超时"},
	}
	got := statsOf(t, items, nodesOf(3), "whoami", false)

	if got.SuccessIPsCount != 1 {
		t.Errorf("SuccessIPsCount = %d, 期望 1", got.SuccessIPsCount)
	}
	if want := []string{"10.0.0.9"}; !slices.Equal(got.SortedSuccessIPs, want) {
		t.Errorf("SortedSuccessIPs = %v, 期望 %v", got.SortedSuccessIPs, want)
	}
	if _, ok := got.CategoryIPMap["执行成功"]; ok {
		t.Errorf("成功分类不该留在 CategoryIPMap 里：%v", got.CategoryIPMap)
	}
	for _, c := range got.SortedFailCategories {
		if c.Category == "执行成功" {
			t.Errorf("成功分类不该进失败分类表：%v", got.SortedFailCategories)
		}
	}
	// 成功行不参与失败分类计数。
	if got.FailCounts != 2 {
		t.Errorf("FailCounts = %d, 期望 2", got.FailCounts)
	}
}

// 失败分类按数量降序；数量相同时保持首次出现的先后（稳定排序）。
func TestStatisticsFailCategoriesSortedByCountDesc(t *testing.T) {
	items := []ssh.Result{
		{IP: "10.0.0.1", Verdict: verdict.Other, Category: "连接超时"},
		{IP: "10.0.0.2", Verdict: verdict.Other, Category: "拒绝网络连接"},
		{IP: "10.0.0.3", Verdict: verdict.Other, Category: "连接超时"},
		{IP: "10.0.0.4", Verdict: verdict.Other, Category: "连接超时"},
		{IP: "10.0.0.5", Verdict: verdict.Other, Category: "原因未知"},
		{IP: "10.0.0.6", Verdict: verdict.Other, Category: "拒绝网络连接"},
		{IP: "10.0.0.7", Verdict: verdict.Other, Category: "原因未知"},
	}
	got := statsOf(t, items, nodesOf(7), "whoami", false)

	want := []CategoryCount{
		{Category: "连接超时", Count: 3},
		{Category: "拒绝网络连接", Count: 2}, // 与「原因未知」同为 2，先出现者在前
		{Category: "原因未知", Count: 2},
	}
	if !reflect.DeepEqual(got.SortedFailCategories, want) {
		t.Errorf("SortedFailCategories = %+v, 期望 %+v", got.SortedFailCategories, want)
	}
}

// 「执行失败(退出码N)」这类分类要把退出码搬进 ExitCode 字段（呈现侧据此不再解名字）；
// 其余分类的 ExitCode 恒为 nil。
func TestStatisticsExitCodeCategoryCarriesExitCode(t *testing.T) {
	items := []ssh.Result{
		{IP: "10.0.0.1", Verdict: verdict.Other, Category: "执行失败(退出码2)", ExitCode: intPtr(2)},
		{IP: "10.0.0.2", Verdict: verdict.Other, Category: "执行失败(退出码2)", ExitCode: intPtr(2)},
		{IP: "10.0.0.3", Verdict: verdict.Other, Category: "连接超时"}, // 无退出码
	}
	got := statsOf(t, items, nodesOf(3), "whoami", false)

	byCategory := map[string]CategoryCount{}
	for _, c := range got.SortedFailCategories {
		byCategory[c.Category] = c
	}

	ec, ok := byCategory["执行失败(退出码2)"]
	if !ok {
		t.Fatalf("缺少分类「执行失败(退出码2)」：%+v", got.SortedFailCategories)
	}
	if ec.Count != 2 {
		t.Errorf("执行失败(退出码2) Count = %d, 期望 2", ec.Count)
	}
	if ec.ExitCode == nil {
		t.Fatalf("执行失败(退出码2) 应带退出码，实际 nil")
	}
	if *ec.ExitCode != 2 {
		t.Errorf("退出码 = %d, 期望 2", *ec.ExitCode)
	}

	other, ok := byCategory["连接超时"]
	if !ok {
		t.Fatalf("缺少分类「连接超时」：%+v", got.SortedFailCategories)
	}
	if other.ExitCode != nil {
		t.Errorf("非退出码分类的 ExitCode 应为 nil，实际 %d", *other.ExitCode)
	}
}

// 分类 IP 图按分类收集 IP，值升序；成功分类不在图里。
func TestStatisticsCategoryIPMap(t *testing.T) {
	items := []ssh.Result{
		{IP: "10.0.0.2", Verdict: verdict.Other, Category: "连接超时"},
		{IP: "10.0.0.1", Verdict: verdict.Other, Category: "连接超时"},
		{IP: "10.0.0.10", Verdict: verdict.Other, Category: "连接超时"},
		{IP: "10.0.0.7", Verdict: verdict.Other, Category: "拒绝网络连接"},
		{IP: "10.0.0.8", Verdict: verdict.Success},
	}
	got := statsOf(t, items, nodesOf(5), "whoami", false)

	want := map[string][]string{
		"连接超时":   {"10.0.0.1", "10.0.0.2", "10.0.0.10"}, // 数值序：10 排在 2 之后
		"拒绝网络连接": {"10.0.0.7"},
	}
	if !maps.EqualFunc(got.CategoryIPMap, want, slices.Equal) {
		t.Errorf("CategoryIPMap = %v, 期望 %v", got.CategoryIPMap, want)
	}
}

// 总数校验三态：齐了「通过」；中断造成缺额置空（不叫「异常」）；其它缺额才是「异常」。
func TestStatisticsVerifyTriState(t *testing.T) {
	threeItems := []ssh.Result{
		{IP: "10.0.0.1", Verdict: verdict.Success},
		{IP: "10.0.0.2", Verdict: verdict.Success},
		{IP: "10.0.0.3", Verdict: verdict.Success},
	}

	t.Run("数目一致为通过", func(t *testing.T) {
		got := statsOf(t, threeItems, nodesOf(3), "whoami", false)
		if got.Verify != "通过" {
			t.Errorf("Verify = %q, 期望 %q", got.Verify, "通过")
		}
		if got.NotExecuted != 0 {
			t.Errorf("NotExecuted = %d, 期望 0", got.NotExecuted)
		}
	})

	t.Run("中断缺额置空", func(t *testing.T) {
		got := statsOf(t, threeItems[:2], nodesOf(3), "whoami", true)
		if got.Verify != "" {
			t.Errorf("Verify = %q, 期望空串（中断不叫异常）", got.Verify)
		}
		if !got.Interrupted {
			t.Errorf("Interrupted 应为 true")
		}
		if got.NotExecuted != 1 {
			t.Errorf("NotExecuted = %d, 期望 1", got.NotExecuted)
		}
	})

	t.Run("非中断缺额为异常", func(t *testing.T) {
		got := statsOf(t, threeItems[:2], nodesOf(3), "whoami", false)
		if got.Verify != "异常" {
			t.Errorf("Verify = %q, 期望 %q", got.Verify, "异常")
		}
		if got.NotExecuted != 1 {
			t.Errorf("NotExecuted = %d, 期望 1", got.NotExecuted)
		}
	})
}

// 全局时间与耗时照传入值算，耗时取秒。
func TestStatisticsGlobalTime(t *testing.T) {
	start := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	stop := start.Add(90 * time.Second)
	got := Statistics(nil, nodesOf(0), argsWith("whoami"), start, stop, false)

	if !got.GlobalStartTime.Equal(start) {
		t.Errorf("GlobalStartTime = %v, 期望 %v", got.GlobalStartTime, start)
	}
	if !got.GlobalStopTime.Equal(stop) {
		t.Errorf("GlobalStopTime = %v, 期望 %v", got.GlobalStopTime, stop)
	}
	if got.GlobalCostTime != 90 {
		t.Errorf("GlobalCostTime = %v, 期望 90", got.GlobalCostTime)
	}
}

// 空结果集：各计数为零，校验按节点数走，分类图建好（非 nil）、可直接读。
func TestStatisticsEmptyResults(t *testing.T) {
	got := statsOf(t, nil, nodesOf(0), "whoami", false)

	if got.ResultsTotal != 0 || got.NodesTotal != 0 {
		t.Errorf("总数应为 0，实际 结果=%d 节点=%d", got.ResultsTotal, got.NodesTotal)
	}
	if got.Verify != "通过" {
		t.Errorf("Verify = %q, 期望 %q", got.Verify, "通过")
	}
	if got.SortedFailCategories != nil {
		t.Errorf("SortedFailCategories 应为空，实际 %+v", got.SortedFailCategories)
	}
	if got.CategoryIPMap == nil {
		t.Fatalf("CategoryIPMap 应初始化成空 map，实际 nil")
	}
	if len(got.CategoryIPMap) != 0 {
		t.Errorf("CategoryIPMap 应为空，实际 %v", got.CategoryIPMap)
	}
	if got.SuccessIPsCount != 0 || len(got.SortedSuccessIPs) != 0 {
		t.Errorf("成功 IP 应为空，实际 数=%d 列表=%v", got.SuccessIPsCount, got.SortedSuccessIPs)
	}
}

// 结果数多于节点数时不炸（N−结果 为负）；这类不一致不是「通过」。
func TestStatisticsMoreResultsThanNodes(t *testing.T) {
	items := []ssh.Result{
		{IP: "10.0.0.1", Verdict: verdict.Success},
		{IP: "10.0.0.2", Verdict: verdict.Success},
	}
	got := statsOf(t, items, nodesOf(1), "whoami", false)

	if got.NotExecuted != -1 {
		t.Errorf("NotExecuted = %d, 期望 -1", got.NotExecuted)
	}
	if got.Verify == "通过" {
		t.Errorf("结果数与节点数不一致，不该判「通过」")
	}
}

// —— 接缝二：SortIPs ——

func TestSortIPs(t *testing.T) {
	cases := []struct {
		name string
		in   []string
		want []string
	}{
		{
			// 数值序而不是字符串序：字符串序会把 10.0.0.10 排到 10.0.0.2 前面。
			name: "按各段数值升序",
			in:   []string{"10.0.0.10", "10.0.0.2", "10.0.0.1", "9.0.0.1", "10.0.1.1"},
			want: []string{"9.0.0.1", "10.0.0.1", "10.0.0.2", "10.0.0.10", "10.0.1.1"},
		},
		{
			name: "已有序保持原样",
			in:   []string{"192.168.1.1", "192.168.1.2"},
			want: []string{"192.168.1.1", "192.168.1.2"},
		},
		{
			name: "空切片",
			in:   []string{},
			want: []string{},
		},
		{
			name: "单元素",
			in:   []string{"10.0.0.1"},
			want: []string{"10.0.0.1"},
		},
		{
			// 非法段按字符串兜底比较：字母段整体排在数字段之后（"a" > "9" 按字符串比）。
			name: "非法段按字符串兜底",
			in:   []string{"10.0.0.1", "10.0.0.a", "10.0.0.9"},
			want: []string{"10.0.0.1", "10.0.0.9", "10.0.0.a"},
		},
		{
			// 段数不同时，短者在前（前缀更短算小）。
			name: "段数不同短者在前",
			in:   []string{"10.0.0.1.5", "10.0.0.1"},
			want: []string{"10.0.0.1", "10.0.0.1.5"},
		},
		{
			// 完全相等的两项，稳定排序保持原序（无法从结果看出，只求不 panic、数量不减）。
			name: "重复项不丢",
			in:   []string{"10.0.0.1", "10.0.0.1"},
			want: []string{"10.0.0.1", "10.0.0.1"},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := SortIPs(c.in)
			if !slices.Equal(got, c.want) {
				t.Errorf("SortIPs(%v) = %v, 期望 %v", c.in, got, c.want)
			}
		})
	}
}

// SortIPs 返回副本：排序不影响调用方手里的原切片。
func TestSortIPsDoesNotMutateInput(t *testing.T) {
	in := []string{"10.0.0.10", "10.0.0.2"}
	before := slices.Clone(in)

	_ = SortIPs(in)

	if !slices.Equal(in, before) {
		t.Errorf("入参被改动：排序前 %v，排序后 %v", before, in)
	}
}

// 统计侧对分类 IP 的排序走的是同一个 SortIPs：图里的值应与直接调 SortIPs 一致。
func TestStatisticsSortsCategoryIPsWithSortIPs(t *testing.T) {
	items := []ssh.Result{
		{IP: "10.0.0.10", Verdict: verdict.Other, Category: "连接超时"},
		{IP: "10.0.0.2", Verdict: verdict.Other, Category: "连接超时"},
	}
	got := statsOf(t, items, nodesOf(2), "whoami", false)

	// 手写字面量：数值序下 10.0.0.2 在 10.0.0.10 之前。
	if want := []string{"10.0.0.2", "10.0.0.10"}; !slices.Equal(got.CategoryIPMap["连接超时"], want) {
		t.Errorf("分类 IP = %v, 期望 %v", got.CategoryIPMap["连接超时"], want)
	}
}
