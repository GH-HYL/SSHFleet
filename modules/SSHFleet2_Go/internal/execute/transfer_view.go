package execute

// 传输类的逐节点进度面板：第一行总进度（条 + 速度 + 已传／总量），第二行节点进度
//（条 + 耗时 + 台数 + 成功失败），一条分隔线，下面最多 20 条逐节点（条 + 速度 + IP + 文件数）。
//
// 形状逐字照老版 `output/progress.go` 的 transferView。命令、脚本、改密仍用原来那一行
//（04 已 A/B 过，不动）。

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/progress"
	tea "github.com/charmbracelet/bubbletea"

	"sshfleet2/internal/contract"
	"sshfleet2/internal/message"
)

const (
	nodeBarWidth    = barWidth
	speedWindowSpan = 2 * time.Second
	separatorWidth  = 50
	maxVisibleNodes = 20
)

// speedSample 滑动窗口里的一个采样点。
type speedSample struct {
	at    time.Time
	bytes int64
}

// speedWindow 滑动窗口测速：按窗口两端的速度差算，不用瞬时值——瞬时值抖得没法看。
type speedWindow struct{ samples []speedSample }

func (w *speedWindow) update(bytes int64) float64 {
	now := time.Now()
	w.samples = append(w.samples, speedSample{now, bytes})
	keep := w.samples[:0]
	for _, s := range w.samples {
		if now.Sub(s.at) <= speedWindowSpan {
			keep = append(keep, s)
		}
	}
	w.samples = keep
	if len(w.samples) < 2 {
		return 0
	}
	first, last := w.samples[0], w.samples[len(w.samples)-1]
	deltaBytes := last.bytes - first.bytes
	deltaTime := last.at.Sub(first.at).Seconds()
	if deltaTime <= 0 || deltaBytes <= 0 {
		return 0
	}
	return float64(deltaBytes) / deltaTime
}

// transferSnapshotMsg 一份传输快照：面板的输入。
type transferSnapshotMsg struct {
	snap snapshot
	mode contract.Mode
}

// nodeView 单个节点在界面上的状态。只记数据，怎么画由 View 现算。
type nodeView struct {
	seq          int
	ip           string
	bytes        int64
	totalBytes   int64
	totalFiles   int
	successFiles int
	failedFiles  int
	done         bool
	speed        float64

	hasBar bool // 是否占着一个显示位（最多 20 个）
	window *speedWindow
}

// transferModel 传输类的面板模型。
type transferModel struct {
	mode  contract.Mode
	total int
	start time.Time

	completed  int
	succeeded  int
	failed     int
	bytesDone  int64
	bytesTotal int64
	totalSpeed float64
	totalBar   progress.Model
	nodeBar    progress.Model

	nodes []*nodeView
	bySeq map[int]*nodeView

	totalWindow *speedWindow
}

func newTransferModel(mode contract.Mode, total int, start time.Time) transferModel {
	return transferModel{
		mode:        mode,
		total:       total,
		start:       start,
		totalBar:    newBar(barWidth),
		nodeBar:     newBar(nodeBarWidth),
		bySeq:       map[int]*nodeView{},
		totalWindow: &speedWindow{},
	}
}

func (m transferModel) Init() tea.Cmd { return nil }

func (m transferModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case transferSnapshotMsg:
		m.apply(msg)
		m.syncBars()
		return m, nil
	case finalMsg:
		return m, tea.Quit
	}
	return m, nil
}

func (m *transferModel) apply(msg transferSnapshotMsg) {
	s := msg.snap
	m.completed, m.succeeded, m.failed = s.Completed, s.Succeeded, s.Failed
	m.bytesDone, m.bytesTotal = s.BytesDone, s.BytesTotal
	m.totalSpeed = m.totalWindow.update(s.BytesDone)

	for _, n := range s.Nodes {
		nv, ok := m.bySeq[n.Seq]
		if !ok {
			nv = &nodeView{seq: n.Seq, window: &speedWindow{}}
			m.bySeq[n.Seq] = nv
			m.nodes = append(m.nodes, nv)
		}
		nv.ip = n.IP
		nv.totalBytes = n.TotalBytes
		nv.totalFiles = n.TotalFiles
		nv.successFiles = n.SuccessFiles
		nv.failedFiles = n.FailedFiles
		nv.done = n.Done
		if n.Bytes > nv.bytes {
			nv.bytes = n.Bytes
		}
		nv.speed = nv.window.update(nv.bytes)
	}
}

// syncBars 维护逐节点条的显示位：已完成的让位，排队中的按序补位，同时显示不超过 20 个。
//
// 显示位只授予「已开始传输（有字节）且未完成」的节点。不按终端高度自适应——
// 72 台满屏 0% 空条观感极差。
func (m *transferModel) syncBars() {
	visible := 0
	for _, n := range m.nodes {
		if n.hasBar && n.done {
			n.hasBar = false
		}
		if n.hasBar {
			visible++
		}
	}
	for _, n := range m.nodes {
		if visible >= maxVisibleNodes {
			break
		}
		if n.hasBar || n.done || n.bytes == 0 {
			continue
		}
		n.hasBar = true
		visible++
	}
}

// progress 总进度：已完成台数 + 各在传节点自身进度之和，除以节点总数。
//
// 不用「已传字节 ÷ 已开始节点的字节总量」：并发受限时前 10 台传完就凑满了分母、把进度
// 顶到 100%，等下一批开始分母变大、进度又跌回来——进度条会「先冲到满再回落」。
// 按台数求和天然反映「整体还剩多少没做完」：还没开始的节点贡献 0。
func (m transferModel) progress() float64 {
	if m.total == 0 {
		return 0
	}
	sum := float64(m.completed)
	for _, n := range m.nodes {
		if !n.done {
			sum += nodePercent(*n)
		}
	}
	return clamp01(sum / float64(m.total))
}

// nodeProgress 节点进度：已完成台数 / 总台数。
func (m transferModel) nodeProgressRatio() float64 {
	if m.total == 0 {
		return 0
	}
	return clamp01(float64(m.completed) / float64(m.total))
}

// finalSnapshot 是收场定格那一帧的输入（06 号单，审计 10-T1）。
//
// 老版跑到末了，屏上留的最后一帧是「节点进度 100%」；新版传输面板与命令那行共用一个流，
// 退出次序把面板压在下面，留下的是「总进度 100%」。收场前把面板再画一帧就补回来了。
//
// **只在真跑完时落满格**：中断的场次 completed 没到总台数，这时候补 100% 就是把「没跑完」
// 画成「跑完了」——那是新的假绿，比缺一帧更坏。所以这一处只判完成度，不判别的。
func finalSnapshot(snap snapshot, total int) snapshot {
	if total <= 0 || snap.Completed < total {
		return snap
	}
	for i := range snap.Nodes {
		snap.Nodes[i].Done = true
	}
	return snap
}

// nodePercent 单节点进度：优先按字节，其次按文件数。
func nodePercent(n nodeView) float64 {
	switch {
	case n.totalBytes > 0:
		return clamp01(float64(n.bytes) / float64(n.totalBytes))
	case n.totalFiles > 0:
		return clamp01(float64(n.successFiles+n.failedFiles) / float64(n.totalFiles))
	default:
		return 0
	}
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func (m transferModel) View() string { return m.render() + "\n" }

// render 界面正文。
func (m transferModel) render() string {
	label := message.ActionName(m.mode) + "进度"

	var b strings.Builder
	fmt.Fprintf(&b, "%s%s  %s  %s  %s/%s\n",
		indent, styleTitle.Render(label), m.totalBar.ViewAs(m.progress()),
		styleDim.Render(message.FormatSpeed(m.totalSpeed)),
		message.Bytes(m.bytesDone), message.Bytes(m.bytesTotal))

	fmt.Fprintf(&b, "%s%s  %s  %s  %d/%d  %s  %s\n",
		indent, styleTitle.Render("节点进度"), m.nodeBar.ViewAs(m.nodeProgressRatio()),
		styleDim.Render(elapsedText(time.Since(m.start))), m.completed, m.total,
		styleOK.Render(fmt.Sprintf("成功 %d", m.succeeded)),
		styleFail.Render(fmt.Sprintf("失败 %d", m.failed)))

	fmt.Fprintf(&b, "%s\n", styleSep.Render(indent+strings.Repeat("─", separatorWidth)))

	for _, n := range m.nodes {
		if !n.hasBar {
			continue
		}
		fmt.Fprintf(&b, "%s%s  %s  %s  %s\n",
			indent, m.nodeBar.ViewAs(nodePercent(*n)), styleDim.Render(message.FormatSpeed(n.speed)),
			styleDim.Render(n.ip),
			fmt.Sprintf("共 %d 个文件  成功 %d  失败 %d", n.totalFiles, n.successFiles, n.failedFiles))
	}
	return strings.TrimRight(b.String(), "\n")
}
