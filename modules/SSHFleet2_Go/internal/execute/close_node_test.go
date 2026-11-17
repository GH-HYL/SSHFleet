package execute

// 一台收口之后，面板该收到什么（2026-11-17 架构复查 01 号单）。
//
// 面板的完成台数与成败靠收口那一次回填；不接这一根线，它就永远是 0，而屏幕照旧
// 画得出来。所以这几条钉的不是「聚合器会不会算」，是「收口有没有把结果交过去」。

import (
	"testing"

	"sshfleet2/internal/contract"
)

func uploadPlan() *contract.Plan {
	return &contract.Plan{Mode: contract.ModeUpload}
}

// 跑成了的一台：完成一台、成功一台，节点标记为已结束（让出显示位靠它）。
func TestCloseNodeFeedsAggregatorOnSuccess(t *testing.T) {
	ag := newAggregator(nil)
	res := &contract.NodeResult{
		IP:        "10.0.0.1",
		Connected: true,
		Transfer:  contract.TransferStat{TotalFiles: 2, SuccessFiles: 2, TotalBytes: 100},
	}

	closeNode(uploadPlan(), 0, res, ag, "")

	snap := ag.snapshotLocked()
	if snap.Completed != 1 || snap.Succeeded != 1 || snap.Failed != 0 {
		t.Fatalf("完成/成功/失败该是 1/1/0，实际 %d/%d/%d", snap.Completed, snap.Succeeded, snap.Failed)
	}
	if len(snap.Nodes) != 1 || !snap.Nodes[0].Done {
		t.Fatalf("这一台该被记成已结束：%+v", snap.Nodes)
	}
	if res.Category != "" && snap.Nodes[0].SuccessFiles != 2 {
		t.Fatalf("面板该看到这一台的文件数，实际 %+v", snap.Nodes[0])
	}
}

// 没成的一台：完成一台、失败一台——失败也要记进总数，否则进度永远差一台。
func TestCloseNodeFeedsAggregatorOnFailure(t *testing.T) {
	ag := newAggregator(nil)
	res := &contract.NodeResult{
		IP:        "10.0.0.2",
		Connected: true,
		Transfer:  contract.TransferStat{TotalFiles: 2, FailFiles: 2},
	}

	closeNode(uploadPlan(), 1, res, ag, "")

	snap := ag.snapshotLocked()
	if snap.Completed != 1 || snap.Succeeded != 0 || snap.Failed != 1 {
		t.Fatalf("完成/成功/失败该是 1/0/1，实际 %d/%d/%d", snap.Completed, snap.Succeeded, snap.Failed)
	}
}

// 非传输路径没有聚合器：收口照常定分类，不许因此在空指针上炸。
func TestCloseNodeWithoutAggregator(t *testing.T) {
	p := &contract.Plan{Mode: contract.ModeCommand}
	res := &contract.NodeResult{IP: "10.0.0.3", Connected: true, ExitCode: intPtr(0)}

	closeNode(p, 0, res, nil, "")

	if res.Category == "" {
		t.Fatal("没有聚合器时分类也该落下来")
	}
}
