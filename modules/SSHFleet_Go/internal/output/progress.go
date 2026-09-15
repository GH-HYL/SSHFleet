// 终端进度条（对位旧 rich 观感）：总进度 + 节点完成进度 + 逐节点进度明细。
// 渲染归 internal/output（spec D2 展开），由 main 注入给 internal/batch 调用。
//
// 与旧版差异：没有 20 个节点的显示上限与"排队机制"——那是 rich 多进度条的限制逼出来的，
// 自写实现按终端高度自适应（spec M5 实现层差异）。
package output

import (
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"sshfleet/internal/batch"
)

const (
	barWidth        = 40
	speedWindowSpan = 2 * time.Second
	separatorWidth  = 50
	indent          = "    "
	// maxVisibleNodes 单节点进度条的显示上限（对位旧版 MAX_VISIBLE_NODES = 20）
	maxVisibleNodes = 20
)

// ANSI 片段（对位 rich 的配色：完成绿 / 结束蓝 / 节点青）
const (
	ansiReset  = "\x1b[0m"
	ansiGreen  = "\x1b[32m"
	ansiCyan   = "\x1b[36m"
	ansiBlue   = "\x1b[34m"
	ansiRed    = "\x1b[31m"
	ansiDim    = "\x1b[90m"
	ansiYellow = "\x1b[33m"
	ansiBGreen = "\x1b[92m"
	ansiBRed   = "\x1b[91m"
)

// ProgressUI 进度呈现器（跨调用保存终端渲染状态；由 batch 的渲染回调逐次驱动）。
type ProgressUI struct {
	out       io.Writer
	mode      string // execute / upload / download
	total     int
	start     time.Time
	mutex     sync.Mutex
	lines     int                  // 上次渲染的行数（用于光标上移重绘）
	lastLines []string             // 上次渲染的内容（PrintAbove 擦除后原样重绘用）
	bars      map[int]bool         // 已获得显示位的节点 seq（传输模式排队机制，上限 maxVisibleNodes）
	speeds    map[int]*speedWindow // 逐节点速度窗口
	total_    *speedWindow         // 总速度窗口
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
		bars:   map[int]bool{},
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

// renderLocked 重绘整个界面：整块擦除旧界面后从原位重绘。
// 不能只覆盖重写——传输模式下节点条完成移除后块会变短，旧块多出的行若不擦除
// 会残留在屏幕上，统计结果打印时叠在半截进度条中间（用户 2026-09-15 指出）。
func (p *ProgressUI) renderLocked(s batch.Snapshot, first bool) {
	lines := p.buildLines(s, first)
	p.clearLocked()
	p.emitLocked(lines)
}

// emitLocked 在当前光标处绘制进度块并记录行数与内容（重绘与 PrintAbove 复用）。
func (p *ProgressUI) emitLocked(lines []string) {
	for _, line := range lines {
		fmt.Fprint(p.out, "\r\x1b[K"+line+"\n")
	}
	p.lines = len(lines)
	p.lastLines = lines
}

// clearLocked 擦除当前进度块：光标上移到块起点，逐行清空后**停回块起点行首**。
//
// 必须停回起点——调用方（renderLocked / PrintAbove）都是从光标处原地覆盖：
// 若停在块的最后一行，每次重绘整块都会下移 N-1 行、并在上方留下 N-1 个空行，
// 传输模式（块有 20+ 行）下空行迅速堆满屏幕、进度条被顶出可视区
// （用户 2026-09-15 反馈「看不到进度条，上面几百个空行」）。
// 命令模式块只有 1 行，起点与末行重合，所以此前未暴露。
func (p *ProgressUI) clearLocked() {
	if p.lines <= 0 {
		return
	}
	fmt.Fprintf(p.out, "\x1b[%dA", p.lines)
	for i := 0; i < p.lines; i++ {
		fmt.Fprint(p.out, "\r\x1b[K")
		if i < p.lines-1 {
			fmt.Fprint(p.out, "\n")
		}
	}
	// 清完光标停在块末行，回退 N-1 行回到块起点（N=1 时已就在起点，且
	// \x1b[0A 在部分终端会被当作上移 1 行，故只在 N>1 时回退）
	if p.lines > 1 {
		fmt.Fprintf(p.out, "\x1b[%dA", p.lines-1)
	}
	p.lines = 0
}

// PrintAbove 在进度界面上方打印外部内容（对位旧 rich Live 的 console.print 行为）：
// 先擦除当前进度块，打印内容，再在内容下方原样重绘进度块——
// 外部输出与进度条各占一块区域、互不覆盖（用户 2026-09-14 要求对齐旧版观感）。
// 多行文本（如单条结果明细）整体作为一个块打印。
func (p *ProgressUI) PrintAbove(text string) {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	p.clearLocked()
	if !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	fmt.Fprint(p.out, text)
	p.emitLocked(p.lastLines)
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

	// 逐节点明细（对位旧版 MAX_VISIBLE_NODES=20 的排队机制）：
	//   已完成的节点释放条；条授予「已开始传输（有字节）且未完成」的节点，
	//   满 maxVisibleNodes 个为止——其余排队不显示，等有节点完成再补位。
	//   不按终端高度自适应：72 台并发时满屏都是 0% 空条，观感极差（用户 2026-09-15 裁定）。
	for _, n := range s.Nodes {
		if n.Done {
			delete(p.bars, n.Seq)
		}
	}
	for _, n := range s.Nodes {
		if len(p.bars) >= maxVisibleNodes {
			break
		}
		if !n.Done && n.Bytes > 0 && !p.bars[n.Seq] {
			p.bars[n.Seq] = true
		}
	}
	for _, n := range s.Nodes {
		if !p.bars[n.Seq] {
			continue
		}
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
