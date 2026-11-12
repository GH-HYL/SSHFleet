package guard

import (
	"testing"
	"time"

	"sshfleet2/internal/contract"
	"sshfleet2/internal/settle"
)

// 红线：结算站的那一件事——递一张第二张纸，回一张总账，字段有人填、附加格原样往下传。
//
// 只判形状与流向：总账里该有值的地方有值、成败加起来等于跑完的台数、附加格原样传回。
// 数对不对（分类排序、逐类 IP 的次序）是结算站自己那一片自定义区的事，这里一条都不判。
func TestRedlineSettleFillsLedger(t *testing.T) {
	start := time.Date(2026, 11, 2, 10, 0, 0, 0, time.UTC)
	paper := contract.ResultPaper{
		Total:     2,
		Extra:     contract.Extra{Mode: contract.ModeCommand},
		StartedAt: start,
		StoppedAt: start.Add(1500 * time.Millisecond),
		Items: []contract.NodeResult{
			{IP: "10.0.0.1", Connected: true, Category: "执行成功"},
			{IP: "10.0.0.2", Connected: true, Category: "执行失败(退出码1)"},
		},
	}

	ledger := settle.Run(paper)

	// 形状与流向，不钉数（ADR-0050）：跑完的加没跑的等于总数、总数有人填。台数对不对
	// 由结算站自己的自定义区判（`settle_test.go` 那一片），这里再判一遍就成了值断言。
	if ledger.Total == 0 || ledger.Done+ledger.NotRun != ledger.Total {
		t.Fatalf("台数那几格没人填或对不上：总 %d、跑完 %d、没跑 %d",
			ledger.Total, ledger.Done, ledger.NotRun)
	}
	if ledger.Verify == "" {
		t.Fatalf("跑没跑齐那一格没人填")
	}
	if ledger.SuccessCounts+ledger.FailCounts != ledger.Done {
		t.Fatalf("成败加起来该等于跑完的台数：%d + %d ≠ %d",
			ledger.SuccessCounts, ledger.FailCounts, ledger.Done)
	}
	if ledger.SuccessCategory == "" {
		t.Fatalf("本模式的成功分类没人填")
	}
	if len(ledger.CategoryIPs) == 0 || len(ledger.SuccessIPs) == 0 {
		t.Fatalf("逐类 IP 与成功 IP 没人填：%+v", ledger.CategoryIPs)
	}
	if ledger.CostTime <= 0 {
		t.Fatalf("总耗时没人填：%v", ledger.CostTime)
	}
	// 附加格原样往下传：结算站可以读它，但不许把它消费掉——报告站还要贴明细
	// （ADR-0035 决定九，2026-11-03 更新记录把「一个字段都不打开」那半句作废了）。
	if ledger.Extra.Mode != contract.ModeCommand {
		t.Fatalf("附加格没原样传：%+v", ledger.Extra)
	}
}
