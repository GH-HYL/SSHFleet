package settle

// 总账的期望值全部写死：这是老版 `result/statistics.go` 同一份输入下的算法，
// 数不是「跑一遍看看出来什么」。

import (
	"testing"
	"time"

	"sshfleet2/internal/contract"
)

// paperOf 造一张第二张纸：命令模式、给定逐台事实、起止时间固定为 2.5 秒。
func paperOf(total int, items []contract.NodeResult) contract.ResultPaper {
	start := time.Date(2026, 11, 2, 10, 0, 0, 0, time.UTC)
	return contract.ResultPaper{
		Total:     total,
		Items:     items,
		Extra:     contract.Extra{Mode: contract.ModeCommand},
		Resources: contract.Resources{Manifest: "u1.csv"},
		StartedAt: start,
		StoppedAt: start.Add(2500 * time.Millisecond),
	}
}

// TestRunSuccessAndFailureAndMissing 三台：一台成功、一台失败、一台没跑。
func TestRunSuccessAndFailureAndMissing(t *testing.T) {
	code := 3
	paper := paperOf(3, []contract.NodeResult{
		{IP: "10.0.0.2", Connected: true, Category: "执行成功"},
		{IP: "10.0.0.1", Connected: true, Category: "执行失败(退出码3)", ExitCode: &code},
	})

	got := Run(paper)

	if got.Total != 3 || got.Done != 2 || got.NotRun != 1 {
		t.Fatalf("台数不对：总 %d、跑完 %d、没跑 %d（该 3/2/1）", got.Total, got.Done, got.NotRun)
	}
	if got.Verify != "异常" {
		t.Fatalf("少跑一台又没中断，校验该是「异常」，实际 %q", got.Verify)
	}
	if got.SuccessCounts != 1 || got.FailCounts != 1 {
		t.Fatalf("成败计数不对：成功 %d、失败 %d（该 1/1）", got.SuccessCounts, got.FailCounts)
	}
	if got.ConnectSuccessCounts != 2 || got.ConnectFailCounts != 0 {
		t.Fatalf("连接计数不对：成功 %d、失败 %d（该 2/0）", got.ConnectSuccessCounts, got.ConnectFailCounts)
	}
	if got.SuccessCategory != "执行成功" {
		t.Fatalf("成功分类该是「执行成功」，实际 %q", got.SuccessCategory)
	}
	if got.CostTime != 2.5 {
		t.Fatalf("总耗时该是 2.5 秒，实际 %v", got.CostTime)
	}

	if len(got.FailCategories) != 1 {
		t.Fatalf("失败分类该只有一类，实际 %d 类：%+v", len(got.FailCategories), got.FailCategories)
	}
	fc := got.FailCategories[0]
	if fc.Category != "执行失败(退出码3)" || fc.Count != 1 {
		t.Fatalf("失败分类不对：%+v", fc)
	}
	if fc.ExitCode == nil || *fc.ExitCode != 3 {
		t.Fatalf("这一类该带退出码 3，实际 %v", fc.ExitCode)
	}

	if len(got.SuccessIPs) != 1 || got.SuccessIPs[0] != "10.0.0.2" {
		t.Fatalf("成功 IP 不对：%v", got.SuccessIPs)
	}
	wantIPs := []contract.CategoryIPs{
		{Category: "执行失败(退出码3)", IPs: []string{"10.0.0.1"}},
		{Category: "执行成功", IPs: []string{"10.0.0.2"}},
	}
	if len(got.CategoryIPs) != len(wantIPs) {
		t.Fatalf("逐类 IP 该有 %d 类，实际 %d 类：%+v", len(wantIPs), len(got.CategoryIPs), got.CategoryIPs)
	}
	for i, want := range wantIPs {
		if got.CategoryIPs[i].Category != want.Category {
			t.Fatalf("第 %d 类该是 %q，实际 %q", i+1, want.Category, got.CategoryIPs[i].Category)
		}
		if len(got.CategoryIPs[i].IPs) != len(want.IPs) || got.CategoryIPs[i].IPs[0] != want.IPs[0] {
			t.Fatalf("第 %d 类的 IP 不对：%v（该 %v）", i+1, got.CategoryIPs[i].IPs, want.IPs)
		}
	}
}

// TestRunCategoryOrder 分类按数量降序；数量相同时按首次出现的先后（屏幕上看得到的次序）。
func TestRunCategoryOrder(t *testing.T) {
	items := []contract.NodeResult{
		{IP: "10.0.0.1", Connected: true, Category: "乙"},
		{IP: "10.0.0.2", Connected: true, Category: "甲"},
		{IP: "10.0.0.3", Connected: true, Category: "甲"},
		{IP: "10.0.0.4", Connected: true, Category: "丙"},
	}
	got := Run(paperOf(4, items))

	want := []string{"甲", "乙", "丙"} // 甲 2 台；乙、丙各 1 台，乙先出现
	if len(got.FailCategories) != len(want) {
		t.Fatalf("该有 %d 类，实际 %d 类：%+v", len(want), len(got.FailCategories), got.FailCategories)
	}
	for i, name := range want {
		if got.FailCategories[i].Category != name {
			t.Fatalf("第 %d 类该是 %q，实际 %q", i+1, name, got.FailCategories[i].Category)
		}
	}
	// 逐类 IP 跟着同一个次序，类内按 IP 数值序。
	if got.CategoryIPs[0].Category != "甲" || len(got.CategoryIPs[0].IPs) != 2 || got.CategoryIPs[0].IPs[0] != "10.0.0.2" {
		t.Fatalf("甲的 IP 不对：%+v", got.CategoryIPs[0])
	}
	if last := got.CategoryIPs[len(got.CategoryIPs)-1]; last.Category != "执行成功" {
		t.Fatalf("成功那一类该排在最后，实际最后一类是 %q", last.Category)
	}
}

// TestRunAllSucceeded 全成功：校验通过、没有失败分类，成功那一类仍列在逐类 IP 里。
func TestRunAllSucceeded(t *testing.T) {
	got := Run(paperOf(2, []contract.NodeResult{
		{IP: "10.0.0.10", Connected: true, Category: "执行成功"},
		{IP: "10.0.0.9", Connected: true, Category: "执行成功"},
	}))

	if got.Verify != "通过" {
		t.Fatalf("一台不缺该是「通过」，实际 %q", got.Verify)
	}
	if got.FailCounts != 0 || len(got.FailCategories) != 0 {
		t.Fatalf("全成功时不该有失败：失败 %d 台、分类 %+v", got.FailCounts, got.FailCategories)
	}
	if len(got.SuccessIPs) != 2 || got.SuccessIPs[0] != "10.0.0.9" {
		t.Fatalf("成功 IP 该按数值序，实际 %v", got.SuccessIPs)
	}
	if len(got.CategoryIPs) != 1 || got.CategoryIPs[0].Category != "执行成功" {
		t.Fatalf("逐类 IP 里该只剩成功那一类：%+v", got.CategoryIPs)
	}
}

// TestRunInterrupted 中断造成的缺额不叫「异常」（那是使用者自己按的）。
func TestRunInterrupted(t *testing.T) {
	paper := paperOf(5, []contract.NodeResult{
		{IP: "10.0.0.1", Connected: true, Category: "执行成功"},
	})
	paper.Interrupted = true

	got := Run(paper)
	if got.Verify != "" {
		t.Fatalf("中断时校验那一格该留空，实际 %q", got.Verify)
	}
	if !got.Interrupted || got.NotRun != 4 {
		t.Fatalf("中断与缺额没记对：中断 %v、没跑 %d（该 true/4）", got.Interrupted, got.NotRun)
	}
}

// TestRunHandsSideStuffThrough 随行件原样往下传：附加格、判据表、运行上下文、执行资源、逐台明细。
func TestRunHandsSideStuffThrough(t *testing.T) {
	paper := paperOf(1, []contract.NodeResult{{IP: "10.0.0.1", Connected: true, Category: "执行成功"}})
	paper.Extra = contract.Extra{Mode: contract.ModeCommand}
	paper.Tables = contract.Tables{Categories: []contract.Category{{Name: "随便一类"}}}
	paper.RunMeta = contract.RunMeta{1, 2, 3}

	got := Run(paper)

	if got.Extra.Mode != contract.ModeCommand {
		t.Fatalf("附加格没原样传：%+v", got.Extra)
	}
	if len(got.Tables.Categories) != 1 || got.Tables.Categories[0].Name != "随便一类" {
		t.Fatalf("判据表没原样传：%+v", got.Tables)
	}
	if string(got.RunMeta) != string([]byte{1, 2, 3}) {
		t.Fatalf("运行上下文没原样传：%v", got.RunMeta)
	}
	if got.Resources.Manifest != "u1.csv" {
		t.Fatalf("执行资源没原样传：%+v", got.Resources)
	}
	if len(got.Items) != 1 || got.Items[0].IP != "10.0.0.1" {
		t.Fatalf("逐台明细没原样传：%+v", got.Items)
	}
}
