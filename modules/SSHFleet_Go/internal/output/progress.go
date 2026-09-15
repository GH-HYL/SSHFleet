// 进度界面（bubbletea + bubbles/progress）。
//
// 为什么换掉原先手写的 ANSI 渲染（用户 2026-09-15 裁定）：
//   - 原实现自己算光标上移、逐行擦除、原地重绘，「进度块整体下移」「上方堆空行」
//     这类 bug 全出在那套手写的光标算术里（progress_test.go 曾专门为它写回归）；
//   - 进度条按整数百分比填充字符，没有动画。
//
// 换成 bubbletea 后：
//   - 「底部固定区域 + 上方自由滚屏」是它的一等公民（View 原地重绘 + tea.Println
//     往上插行），正对旧版 rich Live 的模型；
//   - bubbles/progress 自带弹簧缓动（harmonica）与逐字符渐变色，动画是现成的；
//   - 光标算术整体消失，不再是本项目要负责的正确性问题。
//
// 职责边界：本文件只管「怎么画」。Program 的生命周期（启动 / 收尾 / 非 TTY 退化）
// 在 reporter.go，主干仍独占执行与退出权。
package output

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/progress"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"sshfleet/internal/batch"
)

const (
	barWidth        = 34
	nodeBarWidth    = barWidth
	speedWindowSpan = 2 * time.Second
	separatorWidth  = 50
	indent          = "    "
	// maxVisibleNodes 单节点进度条的显示上限（对位旧版 MAX_VISIBLE_NODES = 20）
	maxVisibleNodes = 20
)

// 界面配色（lipgloss）：标签青、成功绿、失败红、分隔线暗灰。
var (
	styleTitle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39"))
	styleDim   = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	styleOK    = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	styleFail  = lipgloss.NewStyle().Foreground(lipgloss.Color("203"))
	styleSep   = lipgloss.NewStyle().Foreground(lipgloss.Color("238"))

	// 百分比文字样式：bubbles 只给字段、没给 Option
	stylePercent = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#F8F8F2"))
)

// newBar 造一条进度条。样式口径（用户在 demo 里选定的「满配」）：
// 细线字符 ━/─、scaled 渐变（颜色随已填充宽度走）、暗空档色、百分比一位小数且加粗。
func newBar(w int) progress.Model {
	m := progress.New(
		progress.WithWidth(w),
		progress.WithSpringOptions(8, 0.7), // 频率越大越跟手，阻尼越小越弹
		progress.WithFillCharacters('━', '─'),
		progress.WithScaledGradient("#00D9FF", "#5A56E0"),
	)
	m.EmptyColor = "#3C3C50"
	m.PercentFormat = " %5.1f%%"
	m.PercentageStyle = stylePercent
	return m
}

type speedSample struct {
	at    time.Time
	bytes int64
}

// speedWindow 滑动窗口测速（对位旧版逐节点 / 总速度的滑动采样）。
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

// nodeView 单个节点在界面上的状态。bar 只在获得显示位时才建——
// 691 台并发若每台都挂一条带动画的进度条，就是 691 个逐帧定时器。
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

	hasBar bool
	bar    progress.Model
	window *speedWindow
}

type progressModel struct {
	mode  string
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

// newProgressModel 建界面模型。start 由调用方给定（主干传 execStart），
// 与统计块的总耗时同源。
func newProgressModel(mode string, total int, start time.Time) progressModel {
	return progressModel{
		mode:        mode,
		total:       total,
		start:       start,
		totalBar:    newBar(barWidth),
		nodeBar:     newBar(nodeBarWidth),
		bySeq:       map[int]*nodeView{},
		totalWindow: &speedWindow{},
	}
}

func (m progressModel) Init() tea.Cmd { return nil }

func (m progressModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case batch.Snapshot:
		m.applySnapshot(msg)
		return m, m.syncBars()

	// 动画帧：各条自己按 spring 插值逼近目标值；FrameMsg 带 id，不匹配的条会自行忽略
	case progress.FrameMsg:
		cmds := make([]tea.Cmd, 0, len(m.nodes)+2)
		nm, c := m.totalBar.Update(msg)
		m.totalBar = nm.(progress.Model)
		cmds = append(cmds, c)
		nm, c = m.nodeBar.Update(msg)
		m.nodeBar = nm.(progress.Model)
		cmds = append(cmds, c)
		for _, n := range m.nodes {
			if !n.hasBar {
				continue
			}
			nb, c := n.bar.Update(msg)
			n.bar = nb.(progress.Model)
			cmds = append(cmds, c)
		}
		return m, tea.Batch(cmds...)
	}
	return m, nil
}

// applySnapshot 收下一份聚合快照：计数、字节、逐节点状态与速度窗口一次更新完。
func (m *progressModel) applySnapshot(s batch.Snapshot) {
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

// syncBars 维护逐节点条的显示位并推进各条进度。
//
// 显示位规则（对位旧版 MAX_VISIBLE_NODES 的排队机制）：已完成的节点让出显示位；
// 显示位只授予「已开始传输（有字节）且未完成」的节点，满 maxVisibleNodes 为止，
// 其余排队不显示，等有节点完成再补位。不按终端高度自适应——72 台满屏 0% 空条
// 观感极差（用户 2026-09-15 裁定）。
func (m *progressModel) syncBars() tea.Cmd {
	var cmds []tea.Cmd

	visible := 0
	for _, n := range m.nodes {
		if n.hasBar && n.done {
			n.hasBar = false // 完成的节点让位
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
		n.bar = newBar(nodeBarWidth)
		n.hasBar = true
		visible++
	}

	// 推进目标值。Percent() 取的是目标值，只在变化时下发——
	// 每次无脑 SetPercent 会刷新动画 tag，把排队中的动画帧全部作废。
	if pct := m.progress(); m.totalBar.Percent() < pct {
		cmds = append(cmds, m.totalBar.SetPercent(pct))
	}
	if m.total > 0 {
		if pct := float64(m.completed) / float64(m.total); m.nodeBar.Percent() < pct {
			cmds = append(cmds, m.nodeBar.SetPercent(pct))
		}
	}
	for _, n := range m.nodes {
		if !n.hasBar {
			continue
		}
		if pct := nodePercent(*n); n.bar.Percent() < pct {
			cmds = append(cmds, n.bar.SetPercent(pct))
		}
	}
	return tea.Batch(cmds...)
}

// progress 总进度：已完成台数 + 各在传节点自身进度之和，除以节点总数。
//
// 为什么不用「已传字节 ÷ 已开始节点的字节总量」：并发受限时（50 台 10 并发），
// 前 10 台传完就凑满了分母、把进度顶到 100%，而另外 40 台还在排队；等下一批开始，
// 分母变大、进度又跌回来——进度条会「先冲到满再回落」（用户 2026-09-15 实测反馈）。
// 按台数求和天然反映「整体还剩多少没做完」：还没开始的节点贡献 0。
func (m progressModel) progress() float64 {
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

// View 渲染整块。
//
// 末尾刻意多带一个换行：bubbletea 退出时会擦掉自己渲染的最后一行
// （standardRenderer.stop 里的 EraseEntireLine，而 flush 把光标留在最后一行行首）。
// 命令模式的进度条只有一行、正好就是那一行，会被整条抹掉。多留一个空行接刀，
// 进度条本身就能像以前那样留在屏幕上（用户 2026-09-15 实测反馈）。
func (m progressModel) View() string { return m.render() + "\n" }

// render 界面正文（不含上面那个替死换行；测试直接断言它）。
func (m progressModel) render() string {
	if m.mode == "upload" || m.mode == "download" {
		return m.transferView()
	}
	return m.commandView()
}

// commandView 命令模式：单行到位。
func (m progressModel) commandView() string {
	return fmt.Sprintf("%s%s  %s  已完成: %d/%d  %s  %s %s",
		indent, styleTitle.Render("执行进度"), m.totalBar.View(),
		m.completed, m.total, styleDim.Render(elapsedText(time.Since(m.start))),
		styleOK.Render(fmt.Sprintf("Succ:%d", m.succeeded)),
		styleFail.Render(fmt.Sprintf("Fail:%d", m.failed)))
}

// transferView 传输模式：总进度 + 节点进度 + 分隔线 + 逐节点条。
func (m progressModel) transferView() string {
	label := "上传进度"
	if m.mode == "download" {
		label = "下载进度"
	}

	var b strings.Builder
	fmt.Fprintf(&b, "%s%s  %s  %s  %s/%s\n",
		indent, styleTitle.Render(label), m.totalBar.View(),
		styleDim.Render(FormatSpeed(m.totalSpeed)),
		FormatBytes(m.bytesDone), FormatBytes(m.bytesTotal))

	fmt.Fprintf(&b, "%s%s  %s  %s  %d/%d  %s %s\n",
		indent, styleTitle.Render("节点进度"), m.nodeBar.View(),
		styleDim.Render(elapsedText(time.Since(m.start))), m.completed, m.total,
		styleOK.Render(fmt.Sprintf("Succ:%d", m.succeeded)),
		styleFail.Render(fmt.Sprintf("Fail:%d", m.failed)))

	fmt.Fprintf(&b, "%s\n", styleSep.Render(indent+strings.Repeat("─", separatorWidth)))

	for _, n := range m.nodes {
		if !n.hasBar {
			continue
		}
		fmt.Fprintf(&b, "%s%s  %s  %s  Total:%d Succ:%d Fail:%d\n",
			indent, n.bar.View(), styleDim.Render(FormatSpeed(n.speed)),
			styleDim.Render(n.ip), n.totalFiles, n.successFiles, n.failedFiles)
	}
	return strings.TrimRight(b.String(), "\n")
}
