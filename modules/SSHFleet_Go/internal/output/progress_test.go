package output

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"sshfleet/internal/batch"
)

// 进度界面的渲染回归。
//
// 换用 bubbletea 之后，「底部固定、上方滚屏、重绘不残留旧块」这些由框架保证，
// 不再是本项目代码的责任——原先那批「块锚点」测试（连带自写的 ANSI 终端模拟器）
// 随实现一并删除。这里只钉住属于我们自己的部分：两种模式渲染成什么、
// 逐节点显示位怎么分配、进度取值按什么口径。

func newTestProgress(mode string, total int) progressModel {
	return newProgressModel(mode, total, time.Now())
}

// 命令模式：单行，含台数、成败与耗时。
func TestCommandViewIsSingleLineWithCounts(t *testing.T) {
	m := newTestProgress("execute", 5)
	m.applySnapshot(batch.Snapshot{Total: 5, Completed: 3, Succeeded: 2, Failed: 1})

	view := m.render()
	for _, want := range []string{"执行进度", "已完成: 3/5", "Succ:2", "Fail:1"} {
		if !strings.Contains(view, want) {
			t.Fatalf("命令模式界面缺少 %q：\n%s", want, view)
		}
	}
	if strings.Contains(view, "\n") {
		t.Fatalf("命令模式正文应只有一行：\n%s", view)
	}
}

// 传输模式：多行块（总进度 / 节点进度 / 分隔线 / 逐节点条）；
// 已完成的节点让出显示位，正在传输的节点才有条。
func TestTransferViewListsActiveNodesOnly(t *testing.T) {
	m := newTestProgress("upload", 3)
	m.applySnapshot(batch.Snapshot{
		Total: 3, Completed: 1, Succeeded: 1, BytesDone: 512, BytesTotal: 1024,
		Nodes: []batch.NodeSnapshot{
			{Seq: 0, IP: "10.0.0.1", Bytes: 512, TotalBytes: 1024, TotalFiles: 4, SuccessFiles: 4, Done: true},
			{Seq: 1, IP: "10.0.0.2", Bytes: 128, TotalBytes: 1024, TotalFiles: 4},
		},
	})
	m.syncBars()

	view := m.render()
	for _, want := range []string{"上传进度", "节点进度", "10.0.0.2", "Total:4"} {
		if !strings.Contains(view, want) {
			t.Fatalf("传输模式界面缺少 %q：\n%s", want, view)
		}
	}
	if strings.Contains(view, "10.0.0.1") {
		t.Fatalf("已完成的节点不该再占显示位：\n%s", view)
	}
	if got := strings.Count(view, "\n") + 1; got < 3 {
		t.Fatalf("传输模式应渲染成多行块，实际 %d 行：\n%s", got, view)
	}
}

// 逐节点条同时最多 maxVisibleNodes 条（其余排队），对位旧版排队机制。
func TestTransferVisibleNodesCapped(t *testing.T) {
	total := maxVisibleNodes + 5
	nodes := make([]batch.NodeSnapshot, 0, total)
	for i := 0; i < total; i++ {
		nodes = append(nodes, batch.NodeSnapshot{
			Seq: i, IP: fmt.Sprintf("10.0.0.%d", i), Bytes: 1, TotalBytes: 10,
		})
	}
	m := newTestProgress("upload", total)
	m.applySnapshot(batch.Snapshot{Total: total, Nodes: nodes})
	m.syncBars()

	visible := 0
	for _, n := range m.nodes {
		if n.hasBar {
			visible++
		}
	}
	if visible != maxVisibleNodes {
		t.Fatalf("同时显示的节点条应上限 %d，实际 %d", maxVisibleNodes, visible)
	}
}

// View 末尾刻意多带一个换行：bubbletea 退出时会擦掉自己渲染的最后一行
// （命令模式的进度条只有一行，正好是那行），留个空行替它接刀，进度条才能留在屏幕上。
func TestViewLeavesSacrificialTrailingLine(t *testing.T) {
	m := newTestProgress("execute", 2)
	view := m.View()
	if !strings.HasSuffix(view, "\n") {
		t.Fatal("View 末尾应留一个换行，用于承接 bubbletea 退出时的擦行")
	}
	if strings.HasSuffix(strings.TrimSuffix(view, "\n"), "\n") {
		t.Fatalf("只该多一个换行，不该多出空行：\n%q", view)
	}
}

// 总进度按台数加权：并发受限时不会因为「当前这批传完」就冲到 100%。
// （用户 2026-09-15 实测：50 台 10 并发上传，进度条先到 100% 再回落。）
func TestProgressWeightsByNodeCount(t *testing.T) {
	m := newTestProgress("upload", 50)
	nodes := make([]batch.NodeSnapshot, 0, 10)
	for i := 0; i < 10; i++ {
		nodes = append(nodes, batch.NodeSnapshot{
			Seq: i, IP: fmt.Sprintf("10.0.0.%d", i), Bytes: 100, TotalBytes: 100, Done: true,
		})
	}
	// 字节口径下这里会算出 1000/1000 = 1.0
	m.applySnapshot(batch.Snapshot{
		Total: 50, Completed: 10, BytesDone: 1000, BytesTotal: 1000, Nodes: nodes,
	})
	if got := m.progress(); got != 0.2 {
		t.Fatalf("10/50 完成时进度应为 0.2（按字节算会得到 1.0），实际 %v", got)
	}

	// 在传的节点按自身进度计入
	m2 := newTestProgress("upload", 4)
	m2.applySnapshot(batch.Snapshot{Total: 4, Completed: 1, Nodes: []batch.NodeSnapshot{
		{Seq: 0, Bytes: 10, TotalBytes: 10, Done: true},
		{Seq: 1, Bytes: 5, TotalBytes: 10},
	}})
	if got := m2.progress(); got != 0.375 {
		t.Fatalf("(1 完成 + 0.5 在传) / 4 应为 0.375，实际 %v", got)
	}

	// 命令模式：完成的台数 / 总数
	c := newTestProgress("execute", 10)
	c.applySnapshot(batch.Snapshot{Total: 10, Completed: 9})
	if got := c.progress(); got != 0.9 {
		t.Fatalf("命令模式 9/10 应为 0.9，实际 %v", got)
	}
}

// 目标值只在变化时下发：每次无脑 SetPercent 会刷新动画 tag，把排队中的动画帧作废。
func TestSyncBarsSkipsUnchangedPercent(t *testing.T) {
	m := newTestProgress("execute", 4)
	m.applySnapshot(batch.Snapshot{Total: 4, Completed: 2})
	if cmd := m.syncBars(); cmd == nil {
		t.Fatal("进度推进时应下发动画命令")
	}
	if got := m.totalBar.Percent(); got != 0.5 {
		t.Fatalf("总进度目标应为 0.5，实际 %v", got)
	}
	if cmd := m.syncBars(); cmd != nil {
		t.Fatal("数据没变时不该重复下发动画命令")
	}
}

func TestClampAndNodePercent(t *testing.T) {
	if clamp01(-0.5) != 0 || clamp01(1.5) != 1 || clamp01(0.25) != 0.25 {
		t.Fatal("clamp01 应把取值收在 [0,1]")
	}
	if got := nodePercent(nodeView{bytes: 25, totalBytes: 100, totalFiles: 4, successFiles: 1}); got != 0.25 {
		t.Fatalf("有字节量时按字节算（0.25），实际 %v", got)
	}
	if got := nodePercent(nodeView{totalFiles: 4, successFiles: 3}); got != 0.75 {
		t.Fatalf("无字节量时按文件数算（0.75），实际 %v", got)
	}
	if got := nodePercent(nodeView{}); got != 0 {
		t.Fatalf("无任何数据应为 0，实际 %v", got)
	}
}

// 空快照（还没收到任何进度）也要渲染出内容，不能是空串。
func TestViewRendersOnEmptySnapshot(t *testing.T) {
	for _, mode := range []string{"execute", "upload", "download"} {
		m := newTestProgress(mode, 0)
		if got := m.render(); got == "" {
			t.Fatalf("%s 模式在空快照下也应渲染出内容", mode)
		}
	}
}
