// 终端进度条（对位旧 rich 观感）：总进度 + 节点完成进度 + 逐节点进度明细。
// 渲染归 internal/output（spec D2 展开），由 main 注入给 internal/batch 调用。
//
// 与旧版差异：没有 20 个节点的显示上限与"排队机制"——那是 rich 多进度条的限制逼出来的，
// 自写实现按终端高度自适应（spec M5 实现层差异）。
package output

import (
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"golang.org/x/term"

	"sshfleet/internal/batch"
)

const (
	barWidth        = 40
	speedWindowSpan = 2 * time.Second
	separatorWidth  = 50
	indent          = "    "
)

// ANSI 片段（对位 rich 的配色：完成绿 / 结束蓝 / 节点青）
const (
	ansiReset  = "\x1b[0m"
	ansiGreen  = "\x1b[32m"
	ansiCyan   = "\x1b[36m"
	ansiBlue   = "\x1b[34m"
	ansiDim    = "\x1b[90m"
	ansiYellow = "\x1b[33m"
	ansiBGreen = "\x1b[92m"
	ansiBRed   = "\x1b[91m"
)

// ProgressUI 进度呈现器（跨调用保存终端渲染状态；由 batch 的渲染回调逐次驱动）。
type ProgressUI struct {
	out    io.Writer
	mode   string // execute / upload / download
	total  int
	start  time.Time
	mutex  sync.Mutex
	lines  int                  // 上次渲染的行数（用于光标上移重绘）
	speeds map[int]*speedWindow // 逐节点速度窗口
	total_ *speedWindow         // 总速度窗口
}

type speedSample struct {
	at    time.Time
	bytes int64
}

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

// NewProgressUI 创建进度呈现器；返回的 Update 可直接作为 batch 的渲染函数。
func NewProgressUI(out io.Writer, mode string, total int) *ProgressUI {
	ui := &ProgressUI{
		out:    out,
		mode:   mode,
		total:  total,
		start:  time.Now(),
		speeds: map[int]*speedWindow{},
		total_: &speedWindow{},
	}
	ui.Start()
	return ui
}

// Start 打印分隔与初始界面（旧版 Live start 的等价物）。
func (p *ProgressUI) Start() {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	p.renderLocked(p.emptySnapshot(), true)
}

// Update 接收聚合快照并重绘（作为 batch.RenderFunc 使用）。
func (p *ProgressUI) Update(s batch.Snapshot) {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	p.renderLocked(s, false)
}

// Stop 收尾：光标落到界面下方，后续输出不再覆盖。
func (p *ProgressUI) Stop() {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	if p.lines > 0 {
		fmt.Fprint(p.out, "\n")
		p.lines = 0
	}
}

func (p *ProgressUI) emptySnapshot() batch.Snapshot {
	return batch.Snapshot{Total: p.total}
}

// renderLocked 重绘整个界面：先上移光标回到界面起点，逐行重写并清行尾。
func (p *ProgressUI) renderLocked(s batch.Snapshot, first bool) {
	lines := p.buildLines(s, first)

	if !first && p.lines > 0 {
		fmt.Fprintf(p.out, "\x1b[%dA", p.lines)
	}
	for _, line := range lines {
		fmt.Fprint(p.out, "\r\x1b[K"+line+"\n")
	}
	p.lines = len(lines)
}

func (p *ProgressUI) buildLines(s batch.Snapshot, first bool) []string {
	if p.mode == "upload" || p.mode == "download" {
		return p.transferLines(s, first)
	}
	return []string{p.commandLine(s, first)}
}

// transferLines 传输模式：总进度 + 节点完成进度 + 分隔线 + 逐节点明细。
func (p *ProgressUI) transferLines(s batch.Snapshot, first bool) []string {
	label := "上传进度"
	if p.mode == "download" {
		label = "下载进度"
	}

	totalBytes := s.BytesTotal
	doneBytes := s.BytesDone
	totalPct := 0
	if totalBytes > 0 {
		totalPct = int(float64(doneBytes) / float64(totalBytes) * 100)
	}
	speed := 0.0
	if !first {
		speed = p.total_.update(doneBytes)
	}

	lines := []string{
		fmt.Sprintf("%s%s  %s %s%3d%%%s  %s  %s/%s",
			indent, label, bar(totalBytes > 0 && totalPct >= 100, totalPct, ansiGreen, ansiBlue), "",
			totalPct, ansiReset, FormatSpeed(speed), FormatBytes(doneBytes), FormatBytes(totalBytes)),
	}

	nodePct := 0
	if s.Total > 0 {
		nodePct = int(float64(s.Completed) / float64(s.Total) * 100)
	}
	lines = append(lines, fmt.Sprintf("%s节点进度  %s %3d%%  %d/%d  %s  %sSucc:%s%d %sFail:%s%d%s",
		indent, bar(s.Completed >= s.Total && s.Total > 0, nodePct, ansiGreen, ansiBlue), nodePct,
		s.Completed, s.Total, elapsedText(time.Since(p.start)),
		ansiBGreen, ansiDim, s.Succeeded, ansiBRed, ansiDim, s.Failed, ansiReset))

	lines = append(lines, indent+strings.Repeat("─", separatorWidth))

	// 逐节点明细：只显示未完成节点，按 Seq 排序，行数按终端高度自适应
	active := make([]batch.NodeSnapshot, 0, len(s.Nodes))
	for _, n := range s.Nodes {
		if !n.Done {
			active = append(active, n)
		}
	}
	sort.Slice(active, func(i, j int) bool { return active[i].Seq < active[j].Seq })

	maxNodes := p.maxNodeLines()
	if len(active) > maxNodes {
		active = active[:maxNodes]
	}
	for _, n := range active {
		pct := 0
		if n.TotalBytes > 0 {
			pct = int(float64(n.Bytes) / float64(n.TotalBytes) * 100)
		} else if n.TotalFiles > 0 {
			pct = int(float64(n.SuccessFiles+n.FailedFiles) / float64(n.TotalFiles) * 100)
		}
		win, ok := p.speeds[n.Seq]
		if !ok {
			win = &speedWindow{}
			p.speeds[n.Seq] = win
		}
		nodeSpeed := 0.0
		if !first {
			nodeSpeed = win.update(n.Bytes)
		}
		lines = append(lines, fmt.Sprintf("%s%s %3d%%  %s  %s  Total:%d Succ:%d Fail:%d",
			indent, bar(pct >= 100, pct, ansiCyan, ansiBlue), pct, FormatSpeed(nodeSpeed),
			n.IP, n.TotalFiles, n.SuccessFiles, n.FailedFiles))
	}
	return lines
}

// commandLine 命令模式：单行节点完成进度。
func (p *ProgressUI) commandLine(s batch.Snapshot, first bool) string {
	pct := 0
	if s.Total > 0 {
		pct = int(float64(s.Completed) / float64(s.Total) * 100)
	}
	desc := "执行进度"
	if s.Total > 0 {
		desc = fmt.Sprintf("执行进度 已完成: %d/%d", s.Completed, s.Total)
	}
	return fmt.Sprintf("%s%s  %s %3d%%  %d/%d  %s  %sSucc:%s%d %sFail:%s%d%s",
		indent, desc, bar(s.Completed >= s.Total && s.Total > 0, pct, ansiGreen, ansiBlue), pct,
		s.Completed, s.Total, elapsedText(time.Since(p.start)),
		ansiBGreen, ansiDim, s.Succeeded, ansiBRed, ansiDim, s.Failed, ansiReset)
}

// bar 生成 40 格条形图：完成部分着色，未完成部分暗色；finished 时完成部分改蓝色（对位 rich）。
func bar(finished bool, pct int, completeColor, finishedColor string) string {
	filled := pct * barWidth / 100
	if filled > barWidth {
		filled = barWidth
	}
	color := completeColor
	if finished {
		color = finishedColor
	}
	return color + strings.Repeat("━", filled) + ansiReset + ansiDim + strings.Repeat("━", barWidth-filled) + ansiReset
}

// maxNodeLines 逐节点明细的可见行数：按终端高度自适应（无法取到高度时给 10 行）。
func (p *ProgressUI) maxNodeLines() int {
	height := 0
	if f, ok := p.out.(*os.File); ok {
		if _, h, err := term.GetSize(int(f.Fd())); err == nil {
			height = h
		}
	}
	if height <= 0 {
		return 10
	}
	// 总进度 1 + 节点进度 1 + 分隔线 1 + 余量 3 → 其余留给逐节点明细
	n := height - 6
	if n < 3 {
		n = 3
	}
	return n
}
