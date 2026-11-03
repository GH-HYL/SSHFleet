package execute

// 传输面板的形状与显示位数（16 号单）。
//
// 面板是老版逐字照搬的，这些断言钉的是「搬过来还是那个样子」：两行汇总、一条分隔线、
// 下面最多 20 条逐节点；总进度按台数求和；速度按 2 秒窗口算。

import (
	"strings"
	"testing"
	"time"

	"sshfleet2/internal/contract"
)

// panelOf 造一块面板，并把一份快照灌进去。
func panelOf(t *testing.T, total int, snap snapshot) transferModel {
	t.Helper()
	m := newTransferModel(contract.ModeUpload, total, time.Now())
	m.apply(transferSnapshotMsg{snap: snap, mode: contract.ModeUpload})
	m.syncBars()
	return m
}

// 两行汇总 + 分隔线 + 每条一行：形状不对，使用者第一眼就看不明白进度在哪。
func TestTransferPanelShape(t *testing.T) {
	m := panelOf(t, 2, snapshot{
		Completed: 1, Succeeded: 1, BytesDone: 2048, BytesTotal: 4096,
		Nodes: []nodeProgress{
			{Seq: 0, IP: "10.0.0.1", Bytes: 1024, TotalBytes: 1024, TotalFiles: 2, SuccessFiles: 2, Done: true},
			{Seq: 1, IP: "10.0.0.2", Bytes: 512, TotalBytes: 1024, TotalFiles: 2, SuccessFiles: 0},
		},
	})

	view := m.render()
	for _, want := range []string{"上传进度", "节点进度", "成功 1", "失败 0", "1/2"} {
		if !strings.Contains(view, want) {
			t.Fatalf("面板缺 %q：\n%s", want, view)
		}
	}
	if !strings.Contains(view, strings.Repeat("─", separatorWidth)) {
		t.Fatalf("缺那条分隔线：\n%s", view)
	}
	// 逐节点那一行：IP 与三个数都在。
	if !strings.Contains(view, "10.0.0.2") || !strings.Contains(view, "共 2 个文件  成功 0  失败 0") {
		t.Fatalf("逐节点那一行不对：\n%s", view)
	}
	// 已完成的那台让出显示位：只剩一台在传，就该只有一行。
	if n := strings.Count(view, "10.0.0."); n != 1 {
		t.Fatalf("已完成节点该让出显示位，实际画了 %d 行：\n%s", n, view)
	}
}

// 同时在显示的节点不超过 20 个；完成的让位、排队中的按序补位。
func TestTransferPanelCapsVisibleNodes(t *testing.T) {
	const nodes = 26
	snap := snapshot{Nodes: make([]nodeProgress, 0, nodes)}
	for i := 0; i < nodes; i++ {
		snap.Nodes = append(snap.Nodes, nodeProgress{
			Seq: i, IP: "10.0.0.1", Bytes: 10, TotalBytes: 100, TotalFiles: 1,
		})
	}
	m := panelOf(t, nodes, snap)
	if got := countBars(m); got != maxVisibleNodes {
		t.Fatalf("同时在显示的节点该封顶 %d 个，实际 %d", maxVisibleNodes, got)
	}

	// 第 0 台完成：它让位，第 21 台（原本排在外面）补进来，仍在显示的仍是 20 个。
	m.apply(transferSnapshotMsg{
		snap: snapshot{Nodes: []nodeProgress{
			{Seq: 0, IP: "10.0.0.1", Bytes: 100, TotalBytes: 100, TotalFiles: 1, SuccessFiles: 1, Done: true},
		}},
		mode: contract.ModeUpload,
	})
	m.syncBars()
	if got := countBars(m); got != maxVisibleNodes {
		t.Fatalf("补位之后仍该是 %d 个，实际 %d", maxVisibleNodes, got)
	}
	if m.bySeq[0].hasBar {
		t.Fatal("已完成的那台还占着显示位")
	}
	if !m.bySeq[maxVisibleNodes].hasBar {
		t.Fatalf("排队中的第 %d 台没补上位", maxVisibleNodes+1)
	}
}

// 总进度按台数求和：还没开始的台贡献 0，不许用「已传 ÷ 已开始」那种会先冲满再回落的算法。
func TestTransferProgressSumsNodes(t *testing.T) {
	m := panelOf(t, 4, snapshot{
		Completed: 1,
		Nodes: []nodeProgress{
			{Seq: 0, Bytes: 100, TotalBytes: 100, Done: true},
			{Seq: 1, Bytes: 50, TotalBytes: 100},
			{Seq: 2, Bytes: 0, TotalBytes: 100},
			{Seq: 3, Bytes: 0, TotalBytes: 100},
		},
	})
	// (1 台完成 + 0.5 台) / 4 台 = 0.375
	if got := m.progress(); got < 0.374 || got > 0.376 {
		t.Fatalf("总进度应是 0.375，实际 %v", got)
	}
}

// 速度按 2 秒滑动窗口算：窗口外的采样点丢掉，只留最近两秒。
func TestSpeedWindowKeepsTwoSeconds(t *testing.T) {
	w := &speedWindow{}
	now := time.Now()
	w.samples = []speedSample{{at: now.Add(-5 * time.Second), bytes: 100}}
	if v := w.update(2000); v != 0 {
		t.Fatalf("窗口外只剩这一个采样点，算不出速度，实际 %v", v)
	}
	if len(w.samples) != 1 {
		t.Fatalf("5 秒前的采样点该被丢掉，实际剩 %d 个", len(w.samples))
	}

	// 再补一个 1 秒前的点：两点相隔 1 秒、差 2000 字节 ⇒ 约 2000 B/s。
	w = &speedWindow{}
	w.samples = []speedSample{
		{at: now.Add(-1 * time.Second), bytes: 1000},
		{at: now, bytes: 3000},
	}
	if v := w.update(3000); v < 1500 || v > 2500 {
		t.Fatalf("窗口内的速度应在 2000 B/s 上下，实际 %v", v)
	}
}

// 进度事件按 200ms 节流，节点完成时立即渲染一帧。
func TestAggregatorThrottlesButForcesOnDone(t *testing.T) {
	var frames int
	ag := newAggregator(func(snapshot) { frames++ })

	ag.onProgress(nodeProgress{Seq: 0, Bytes: 10, TotalBytes: 100})
	ag.onProgress(nodeProgress{Seq: 0, Bytes: 20, TotalBytes: 100})
	ag.onProgress(nodeProgress{Seq: 1, Bytes: 10, TotalBytes: 100})
	if frames != 1 {
		t.Fatalf("200ms 内的连续进度事件该只渲染一帧，实际 %d 帧", frames)
	}

	ag.onResult(0, "10.0.0.1", true, contract.TransferStat{TotalBytes: 100, TotalFiles: 1, SuccessFiles: 1})
	if frames != 2 {
		t.Fatalf("节点完成该立即渲染一帧，实际共 %d 帧", frames)
	}
}

// countBars 数一下有多少台占着显示位。
func countBars(m transferModel) int {
	n := 0
	for _, v := range m.nodes {
		if v.hasBar {
			n++
		}
	}
	return n
}
