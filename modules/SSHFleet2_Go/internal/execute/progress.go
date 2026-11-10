package execute

// 进度界面：跑的过程中屏幕上那块「跑到哪儿了」。
//
// 形状照老版 `output/progress.go`：底部固定一块（总进度条、已完成台数、耗时、成功失败数），
// 上面自由滚结果行——`bubbletea` 的 `Println` 往上插行、`View` 原地重绘，正是老版那套观感。
//
// **屏幕的流从 `console` 拿**（`console.Writer`），执行站是唯一可以拿它的包；画帧与结果行
// 走同一条流，才不会互相盖。字本身仍只有两个来源：结果行由 `message` 造，进度帧的标签是
// 面板的一部分（老版也住在呈现层，不在句子那一处）。
//
// 非终端（重定向、管道）不出界面，结果行直接落流——老版按 `isatty(os.Stdout)` 判同一件事。

import (
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/bubbles/progress"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"sshfleet2/internal/console"
	"sshfleet2/internal/contract"
)

const (
	barWidth = 34
	indent   = "    "
)

// 面板配色（对位老版）：标签青、暗灰、成功绿、失败红。
//
// 值与原版逐字相同：低色彩终端降色之后仍分得开（老版 2026-10-07 在 demo 里选定）。
var (
	styleTitle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39"))
	styleDim     = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	styleOK      = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	styleFail    = lipgloss.NewStyle().Foreground(lipgloss.Color("203"))
	styleSep     = lipgloss.NewStyle().Foreground(lipgloss.Color("238"))
	stylePercent = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#F8F8F2"))
)

// snapshotMsg 一份进度快照：跑到第几台、成了几台、败了几台。
type snapshotMsg struct {
	completed int
	succeeded int
	failed    int
}

// finalMsg 收尾消息：收工时下发一次，由 Update 返回 tea.Quit。
//
// 走消息而不直接 Quit：最后那几条结果行与本消息走同一条队列，事件循环按序处理，退出一定
// 排在它们渲染之后——界面不会被提前抹掉（老版同一手）。
type finalMsg struct{}

// progressModel 进度面板。命令模式只有一行：总进度条 + 台数 + 耗时 + 成功失败。
//
// 传输类那种逐节点面板随阶段 05（老版分两个视图，命令与脚本、改密共用一个）。
type progressModel struct {
	total int
	start time.Time

	completed int
	succeeded int
	failed    int

	bar progress.Model
}

func newProgressModel(total int, start time.Time) progressModel {
	return progressModel{total: total, start: start, bar: newBar(barWidth)}
}

// newBar 造一条进度条：细线字符 ━/─、scaled 渐变（颜色随已填充宽度走）、暗空档色、
// 百分比一位小数且加粗。
//
// 不挂弹簧动画：批量执行时快照比动画帧还密，弹簧会被一次次重设目标饿死（老版真机实测：
// 几百台跑完仍显示 0.0%）。按目标值直出（ViewAs），数字永远等于真实进度。
func newBar(w int) progress.Model {
	m := progress.New(
		progress.WithWidth(w),
		progress.WithFillCharacters('━', '─'),
		// 渐变两端：#C05050（暗红）→ #FFB454（亮橙）。低色彩终端降色后仍分得出两端。
		progress.WithScaledGradient("#C05050", "#FFB454"),
	)
	m.EmptyColor = "#3C3C50"
	m.PercentFormat = " %5.1f%%"
	m.PercentageStyle = stylePercent
	return m
}

func (m progressModel) Init() tea.Cmd { return nil }

// Update 只认两类消息：快照改数据、收尾退场。没有任何动画帧，所以不回吐命令。
func (m progressModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case snapshotMsg:
		m.completed, m.succeeded, m.failed = msg.completed, msg.succeeded, msg.failed
		return m, nil
	case finalMsg:
		return m, tea.Quit
	}
	return m, nil
}

// View 渲染整块，末尾多带一个换行。
//
// 那个换行是接刀的：bubbletea 退出时会擦掉它渲染的最后一行，而命令模式的进度条只有一行、
// 正好就是那一行，不多留一行就被整条抹掉（老版 2026-09-15 实测反馈）。
func (m progressModel) View() string { return m.render() + "\n" }

func (m progressModel) render() string {
	return fmt.Sprintf("%s%s  %s  已完成 %d/%d  %s  %s  %s",
		indent, styleTitle.Render("总进度"), m.bar.ViewAs(m.percent()),
		m.completed, m.total, styleDim.Render(elapsedText(time.Since(m.start))),
		styleOK.Render(fmt.Sprintf("成功 %d", m.succeeded)),
		styleFail.Render(fmt.Sprintf("失败 %d", m.failed)))
}

// percent 总进度：已完成台数 / 总台数。没开工时是 0，不会先冲满再回落。
func (m progressModel) percent() float64 {
	if m.total <= 0 {
		return 0
	}
	p := float64(m.completed) / float64(m.total)
	if p > 1 {
		return 1
	}
	return p
}

// elapsedText 耗时写成 `0:00:12`（老版同一格式）。
func elapsedText(d time.Duration) string {
	total := int(d.Seconds() + 0.5)
	return fmt.Sprintf("%d:%02d:%02d", total/3600, (total%3600)/60, total%60)
}

// screen 屏幕那一路：一个流，加一个懒起来的进度界面。
//
// 懒起来是为了让「采集期提示先落终端」（老版 2026-09-14 裁定）：第一个进度快照之前，
// 结果行与提示都直接落流，不先铺一屏界面。
type screen struct {
	mu    sync.Mutex
	w     io.Writer
	prog  *tea.Program // 命令／脚本／改密那块单行面板
	tprog *tea.Program // 传输类那块逐节点面板（两种模式一执行期只会起一种）
	plain bool

	// 传输面板收场定格用（06 号单）：存最后一份快照与它的模式／总台数。
	// 两块界面共用一个流，退出次序决定屏上留谁——老版留的是「节点进度 100%」，
	// 新版留成了「总进度 100%」（审计 10-T1）。收场前拿这份快照再画一帧就补回来了。
	lastSnap  snapshot
	lastMode  contract.Mode
	lastTotal int
	hasSnap   bool
}

// newScreen 造屏幕那一路。流由 `console` 发，是不是终端也由它判。
func newScreen(w io.Writer) *screen {
	return &screen{w: w, plain: !console.IsTerminal()}
}

// above 把一段字打在进度界面上方：界面起来了走它的 `Println`，没起来（非终端、或还没有
// 进度快照）直接写流。**同一段字，不另排一遍**——屏幕上与落盘的那一段必须一样。
func (s *screen) above(text string) {
	s.mu.Lock()
	prog := s.prog
	s.mu.Unlock()

	if prog == nil {
		_, _ = io.WriteString(s.w, text) // 给人看的那一段自带结尾换行
		return
	}
	// Println 自己补一个换行，正文末尾那个就不必再带。
	prog.Println(strings.TrimSuffix(text, "\n"))
}

// transfer 一份传输快照：懒起那块逐节点面板（与命令那行同一套懒法）。
func (s *screen) transfer(snap snapshot, mode contract.Mode, total int, start time.Time) {
	if s.plain {
		return
	}

	s.mu.Lock()
	// 存下最后一份快照：收场那一步要拿它定格（见 stop）。
	s.lastSnap, s.lastMode, s.lastTotal, s.hasSnap = snap, mode, total, true
	if s.tprog == nil {
		p := tea.NewProgram(newTransferModel(mode, total, start),
			tea.WithOutput(s.w),
			tea.WithInput(nil),
			tea.WithoutSignalHandler(),
		)
		go func() { _, _ = p.Run() }()
		s.tprog = p
	}
	prog := s.tprog
	s.mu.Unlock()

	prog.Send(transferSnapshotMsg{snap: snap, mode: mode})
}

// progress 一份进度快照。非终端不画界面（与老版同一判据）。
func (s *screen) progress(completed, succeeded, failed, total int, start time.Time) {
	if s.plain {
		return
	}

	s.mu.Lock()
	if s.prog == nil {
		p := tea.NewProgram(newProgressModel(total, start),
			tea.WithOutput(s.w),
			tea.WithInput(nil),         // 不接输入：中断由信号负责
			tea.WithoutSignalHandler(), // 不抢 SIGINT，免得与塔尖的信号处理打架
		)
		// Run 是阻塞的，必须自己一个 goroutine；Send 会在事件循环就绪前自动等。
		go func() { _, _ = p.Run() }()
		s.prog = p
	}
	prog := s.prog
	s.mu.Unlock()

	prog.Send(snapshotMsg{completed: completed, succeeded: succeeded, failed: failed})
}

// stop 收工：让界面把最后那几帧与结果行渲染完再退，光标落回界面下方。幂等。
//
// 传输面板那一块要多一步：退出前把最后一份快照再画一帧（06 号单，审计 10-T1）。
// 两块界面共用一个流，谁后退出就留谁——老版屏上留的是「节点进度 100%」，新版留成了
// 命令那行的「总进度 100%」。补这一帧与「动画节奏不同」不是一回事，它是关键行变了。
func (s *screen) stop() {
	s.mu.Lock()
	prog, tprog := s.prog, s.tprog
	snap, mode, total, hasSnap := s.lastSnap, s.lastMode, s.lastTotal, s.hasSnap
	s.prog, s.tprog = nil, nil
	s.hasSnap = false
	s.mu.Unlock()

	if prog != nil {
		prog.Send(finalMsg{})
		prog.Wait()
	}
	if tprog != nil {
		if hasSnap {
			tprog.Send(transferSnapshotMsg{snap: finalSnapshot(snap, total), mode: mode})
		}
		tprog.Send(finalMsg{})
		tprog.Wait()
	}
}
