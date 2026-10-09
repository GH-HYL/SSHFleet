package output

import (
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"sshfleet/internal/batch"
	"sshfleet/internal/cli"
	"sshfleet/internal/ssh"
)

// 进度界面的渲染回归。
//
// 换用 bubbletea 之后，「底部固定、上方滚屏、重绘不残留旧块」这些由框架保证，
// 不再是本项目代码的责任——原先那批「块锚点」测试（连带自写的 ANSI 终端模拟器）
// 随实现一并删除。这里只钉住属于我们自己的部分：两种模式渲染成什么、
// 逐节点显示位怎么分配、进度取值按什么口径。

func newTestProgress(mode cli.Mode, total int) progressModel {
	return newProgressModel(mode, total, time.Now())
}

// 命令模式：单行，含台数、成败与耗时。
func TestCommandViewIsSingleLineWithCounts(t *testing.T) {
	m := newTestProgress(cli.ModeCommand, 5)
	m.applySnapshot(batch.Snapshot{Completed: 3, Succeeded: 2, Failed: 1})

	view := m.render()
	for _, want := range []string{"总进度", "已完成 3/5", "成功 2", "失败 1"} {
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
	m := newTestProgress(cli.ModeUpload, 3)
	m.applySnapshot(batch.Snapshot{
		Completed: 1, Succeeded: 1, BytesDone: 512, BytesTotal: 1024,
		Nodes: []batch.NodeSnapshot{
			{Seq: 0, IP: "10.0.0.1", Bytes: 512, TotalBytes: 1024, TotalFiles: 4, SuccessFiles: 4, Done: true},
			{Seq: 1, IP: "10.0.0.2", Bytes: 128, TotalBytes: 1024, TotalFiles: 4},
		},
	})
	m.syncBars()

	view := m.render()
	for _, want := range []string{"上传进度", "节点进度", "10.0.0.2", "共 4 个文件"} {
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
	m := newTestProgress(cli.ModeUpload, total)
	m.applySnapshot(batch.Snapshot{Nodes: nodes})
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
	m := newTestProgress(cli.ModeCommand, 2)
	view := m.View()
	if !strings.HasSuffix(view, "\n") {
		t.Fatal("View 末尾应留一个换行，用于承接 bubbletea 退出时的擦行")
	}
	if strings.HasSuffix(strings.TrimSuffix(view, "\n"), "\n") {
		t.Fatalf("只该多一个换行，不该多出空行：\n%q", view)
	}
}

// 命令模式的进度序列必须单调不减：用真实的聚合器喂一串「逐个节点完成」的快照，
// 逐步打印进度值。（用户 2026-09-15 反馈看到「先 100% 又回落」，这里先把数据层钉死。）
func TestExecuteProgressSequenceMonotonic(t *testing.T) {
	total := 8
	agg := batch.NewAggregator(nil)
	m := newTestProgress(cli.ModeCommand, total)

	m.applySnapshot(agg.Snapshot())
	t.Logf("初始        : %.4f", m.progress())

	last := m.progress()
	for i := 0; i < total; i++ {
		agg.OnResult(ssh.Result{
			Seq: i, IP: fmt.Sprintf("10.0.0.%d", i+1),
			ConnectSuccess: true, ExitCode: intPtr(0),
		})
		m.applySnapshot(agg.Snapshot())
		got := m.progress()
		t.Logf("第 %d 台完成: %.4f", i+1, got)
		if got < last {
			t.Fatalf("第 %d 台完成后进度回落：%.4f → %.4f", i+1, last, got)
		}
		last = got
	}
	if last != 1 {
		t.Fatalf("全部完成后进度应为 1，实际 %v", last)
	}
}

// 条与百分比恒等于目标进度——不落后、不等动画，第一条快照进来就位。
// （用户 2026-10-08 真机：1109 台里几百台跑完了，条还钉在 0.0%。根因是弹簧动画的帧按
// tag 认领，快照比帧密时每帧都被下一次重设目标作废，显示值完全冻结；改为直出后不存在。）
func TestBarsRenderTargetImmediately(t *testing.T) {
	m := newTestProgress(cli.ModeCommand, 4)
	m.applySnapshot(batch.Snapshot{Completed: 4})

	if got := m.render(); !strings.Contains(got, "100.0%") {
		t.Fatalf("命令模式的条与百分比应立刻等于目标值 100.0%%，实际：\n%s", got)
	}

	// 传输模式的三条（总进度 / 节点进度 / 逐节点）同样直出
	tr := newTestProgress(cli.ModeUpload, 2)
	tr.applySnapshot(batch.Snapshot{
		Completed: 2, BytesDone: 100, BytesTotal: 100,
		Nodes: []batch.NodeSnapshot{{Seq: 0, IP: "10.0.0.1", Bytes: 50, TotalBytes: 50, SuccessFiles: 1, TotalFiles: 1}},
	})
	tr.syncBars()
	if got := tr.render(); !strings.Contains(got, "100.0%") {
		t.Fatalf("传输模式的条也应立刻等于目标值 100.0%%，实际：\n%s", got)
	}
}

// 收尾消息要带回退出命令（走消息而不是直接 Quit，好让先前入队的渲染先落屏）。
func TestFinalMsgQuits(t *testing.T) {
	m := newTestProgress(cli.ModeCommand, 2)
	if _, cmd := m.Update(finalMsg{}); cmd == nil {
		t.Fatal("终帧消息应带回退出命令")
	}
}

// 总进度按台数加权：并发受限时不会因为「当前这批传完」就冲到 100%。
// （用户 2026-09-15 实测：50 台 10 并发上传，进度条先到 100% 再回落。）
func TestProgressWeightsByNodeCount(t *testing.T) {
	m := newTestProgress(cli.ModeUpload, 50)
	nodes := make([]batch.NodeSnapshot, 0, 10)
	for i := 0; i < 10; i++ {
		nodes = append(nodes, batch.NodeSnapshot{
			Seq: i, IP: fmt.Sprintf("10.0.0.%d", i), Bytes: 100, TotalBytes: 100, Done: true,
		})
	}
	// 字节口径下这里会算出 1000/1000 = 1.0
	m.applySnapshot(batch.Snapshot{
		Completed: 10, BytesDone: 1000, BytesTotal: 1000, Nodes: nodes,
	})
	if got := m.progress(); got != 0.2 {
		t.Fatalf("10/50 完成时进度应为 0.2（按字节算会得到 1.0），实际 %v", got)
	}

	// 在传的节点按自身进度计入
	m2 := newTestProgress(cli.ModeUpload, 4)
	m2.applySnapshot(batch.Snapshot{Completed: 1, Nodes: []batch.NodeSnapshot{
		{Seq: 0, Bytes: 10, TotalBytes: 10, Done: true},
		{Seq: 1, Bytes: 5, TotalBytes: 10},
	}})
	if got := m2.progress(); got != 0.375 {
		t.Fatalf("(1 完成 + 0.5 在传) / 4 应为 0.375，实际 %v", got)
	}

	// 命令模式：完成的台数 / 总数
	c := newTestProgress(cli.ModeCommand, 10)
	c.applySnapshot(batch.Snapshot{Completed: 9})
	if got := c.progress(); got != 0.9 {
		t.Fatalf("命令模式 9/10 应为 0.9，实际 %v", got)
	}
}

// 快照消息不该回吐命令：条是直出的，渲染不再需要"再转一圈"的事件循环。
func TestSnapshotUpdateIssuesNoCommand(t *testing.T) {
	m := newTestProgress(cli.ModeCommand, 4)
	if _, cmd := m.Update(batch.Snapshot{Completed: 2}); cmd != nil {
		t.Fatal("快照处理不该产生后续命令（直出渲染没有动画帧）")
	}
}

// 显示值跟得上真实进度：快照来得比动画帧还密（1ms 一份，远快于 16.7ms 的帧）时也不许落后。
// 这是「几百台跑完、条还停在 0.0%」那个真机问题的回归位——旧实现此时会一直停在 0.0%。
func TestDisplayTracksProgressUnderBurst(t *testing.T) {
	const total = 300
	m := newTestProgress(cli.ModeCommand, total)
	for i := 1; i <= total; i++ {
		updated, _ := m.Update(batch.Snapshot{Completed: i})
		m = updated.(progressModel)
		if i%50 == 0 {
			got := percentIn(t, m.render())
			want := float64(i) / total * 100
			if got < want-0.1 {
				t.Fatalf("第 %d/%d 台完成时显示值落后：显示 %.1f%%，真实 %.1f%%", i, total, got, want)
			}
		}
		time.Sleep(time.Millisecond)
	}
}

// 显示值不得回退：进度值是跳变的（一次可能同时完成好几台），旧实现靠弹簧的临界阻尼保证
// 「数字只朝一个方向走」；现在由直出 + progress() 单调保证。
func TestDisplayedPercentNeverGoesBackwards(t *testing.T) {
	const total = 40
	m := newTestProgress(cli.ModeCommand, total)
	last := 0.0
	for i := 0; i <= total; i++ {
		updated, _ := m.Update(batch.Snapshot{Completed: i})
		m = updated.(progressModel)
		got := percentIn(t, m.render())
		if got < last {
			t.Fatalf("第 %d 台完成后显示值回退：%.1f%% → %.1f%%", i, last, got)
		}
		last = got
	}
	if last != 100 {
		t.Fatalf("全部完成后显示值应为 100%%，实际 %.1f%%", last)
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
	for _, mode := range []cli.Mode{cli.ModeCommand, cli.ModeUpload, cli.ModeDownload} {
		m := newTestProgress(mode, 0)
		if got := m.render(); got == "" {
			t.Fatalf("%s 模式在空快照下也应渲染出内容", mode)
		}
	}
}

// 条按目标值直出：给多少就画多少，不截断也不溢出（进度取值离开 [0,1] 时由 clamp01 收住）。
func TestBarRendersExactlyGivenPercent(t *testing.T) {
	for _, pct := range []float64{0, 0.375, 1} {
		got := newBar(24).ViewAs(pct)
		want := fmt.Sprintf("%.1f%%", pct*100)
		if !strings.HasSuffix(got, want) {
			t.Fatalf("ViewAs(%.3f) 应显示 %s，实际 %q", pct, want, got)
		}
	}
}

// percentIn 从渲染出来的一行里读百分比数字（条与百分比同源，取最后一个）。
func percentIn(t *testing.T, view string) float64 {
	t.Helper()
	i := strings.LastIndexByte(view, '%')
	if i <= 0 {
		t.Fatalf("渲染结果里找不到百分比：%q", view)
	}
	j := i - 1
	for j >= 0 && ((view[j] >= '0' && view[j] <= '9') || view[j] == '.') {
		j--
	}
	f, err := strconv.ParseFloat(view[j+1:i], 64)
	if err != nil {
		t.Fatalf("解析百分比失败（%q）：%q", view[j+1:i], view)
	}
	return f
}
