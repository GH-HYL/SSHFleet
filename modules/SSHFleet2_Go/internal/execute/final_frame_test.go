package execute

// 收场定格那一帧（06 号单，审计 10-T1：「节点进度」缺 100% 终帧）。
//
// 判据两条：真跑完时节点进度落满格；中断时原样交回、一台都不许替它标完成。
// 第二条比第一条要紧——补错方向会把「没跑完」画成「跑完了」。

import (
	"testing"
	"time"

	"sshfleet2/internal/contract"
)

// 真跑完：每一台都标成完成，节点进度与总进度都满格。
func TestFinalSnapshotMarksAllDoneWhenComplete(t *testing.T) {
	snap := snapshot{
		Completed: 2, Succeeded: 2,
		Nodes: []nodeProgress{
			{Seq: 0, IP: "10.0.0.1", Bytes: 100, TotalBytes: 100, Done: true},
			{Seq: 1, IP: "10.0.0.2", Bytes: 100, TotalBytes: 100},
		},
	}

	got := finalSnapshot(snap, 2)
	if got.Completed != 2 {
		t.Fatalf("完成台数不该被改：%d", got.Completed)
	}
	for _, n := range got.Nodes {
		if !n.Done {
			t.Fatalf("真跑完时每一台都该标成已完成：%+v", n)
		}
	}

	// 落到面板上：节点进度满格；总进度也满格（按台数求和，已完成台数抵满总台数）。
	m := newTransferModel(contract.ModeUpload, 2, time.Now())
	m.apply(transferSnapshotMsg{snap: got, mode: contract.ModeUpload})
	if r := m.nodeProgressRatio(); r != 1 {
		t.Fatalf("收场那一帧的节点进度该是 1.0，实际 %v", r)
	}
	if p := m.progress(); p != 1 {
		t.Fatalf("总进度也该满格，实际 %v", p)
	}
}

// 中断：原样交回，一台都不许替它标完成。
func TestFinalSnapshotLeavesInterruptedAsIs(t *testing.T) {
	snap := snapshot{
		Completed: 1,
		Nodes: []nodeProgress{
			{Seq: 0, IP: "10.0.0.1", Bytes: 100, TotalBytes: 100, Done: true},
			{Seq: 1, IP: "10.0.0.2", Bytes: 40, TotalBytes: 100},
			{Seq: 2, IP: "10.0.0.3"}, // 压根没开跑
		},
	}

	got := finalSnapshot(snap, 3)
	if got.Completed != 1 {
		t.Fatalf("中断时完成台数不该动：%d", got.Completed)
	}
	if got.Nodes[1].Done || got.Nodes[2].Done {
		t.Fatalf("中断时不许替没跑完的台标完成：%+v", got.Nodes)
	}

	m := newTransferModel(contract.ModeUpload, 3, time.Now())
	m.apply(transferSnapshotMsg{snap: got, mode: contract.ModeUpload})
	if r := m.nodeProgressRatio(); r >= 1 {
		t.Fatalf("中断的场次节点进度不该满格，实际 %v", r)
	}
}

// 空跑（0 台）还是「完成数已抵到总台数」这类怪输入：都不许 panic，按上面两条口径处理。
func TestFinalSnapshotHandlesEdgeCases(t *testing.T) {
	empty := finalSnapshot(snapshot{}, 0)
	if empty.Completed != 0 || len(empty.Nodes) != 0 {
		t.Fatalf("空快照该原样：%+v", empty)
	}

	over := snapshot{Completed: 3, Nodes: []nodeProgress{{Seq: 0}, {Seq: 1}}}
	got := finalSnapshot(over, 2)
	for _, n := range got.Nodes {
		if !n.Done {
			t.Fatalf("完成数已抵到总台数，节点该落满格：%+v", got.Nodes)
		}
	}
}
