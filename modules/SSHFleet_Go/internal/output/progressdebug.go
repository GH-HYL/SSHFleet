// 进度链路的时间戳记录（排障用，对应 docs/issues/progress-update-lag）。
//
// 为什么单独一处：进度条滞后要判的是「谁在等谁」，而这条链路横跨两个 goroutine 与一段
// 库内代码——快照由收集循环投递（阻塞地送进 bubbletea 的无缓冲消息队列），事件循环再逐条
// Update + View。任何一段慢了都表现为「进度不动」，只有把两端的时刻分别记下来才分得清
// 是投递在等、还是渲染在慢。
//
// 这里只记数、不改行为：不加缓冲、不丢消息、不动渲染节奏——排障记录不该让被观察的东西变样。
// 全部落 DEBUG；只在有进度界面时才有内容（直通模式没有可记的链路）。
package output

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"sshfleet/internal/batch"
	"sshfleet/internal/log"
)

// progressStats 进度链路两侧的计时。
//
// 投递侧（进展度界面方法 / 明细行的那个 goroutine——命令模式是单线程的收集循环，
// 传输模式还会有 worker 的字节级回调）用一把锁；事件循环侧（bubbletea 单线程的
// Update / View）用原子量——那是最热的一段，不该再回到锁上排队。
type progressStats struct {
	el *log.Logger

	mu       sync.Mutex
	snapSent int
	snapSum  time.Duration
	snapMax  time.Duration
	lineSent int
	lineSum  time.Duration
	lineMax  time.Duration

	msgs      atomic.Int64 // 事件循环处理过的消息条数
	frames    atomic.Int64 // View 被调用次数（= 渲染帧数）
	viewNanos atomic.Int64 // View 累计耗时
	viewMaxNS atomic.Int64 // 单帧最长
}

// newProgressStats 建一份计时记录；el 可为 nil（只计数、不写日志）。
func newProgressStats(el *log.Logger) *progressStats { return &progressStats{el: el} }

// recordSnapSend 记一次进度快照投递。wait 是「交到事件循环手上」用的时间——Send 是阻塞发，
// 这个数就是事件循环腾出手来收下这条消息花了多久。快照自带已完成数与节点数，一并写进日志，
// 好把某一刻的等待对回到当时的进度上。
func (s *progressStats) recordSnapSend(wait time.Duration, snap batch.Snapshot) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.snapSent++
	s.snapSum += wait
	if wait > s.snapMax {
		s.snapMax = wait
	}
	seq := s.snapSent
	s.mu.Unlock()
	if s.el != nil {
		s.el.Debug(fmt.Sprintf("进度投递：第 %d 份快照（已完成 %d/%d，带节点 %d 个）交到事件循环手上用了 %.3fs",
			seq, snap.Completed, snap.Total, len(snap.Nodes), wait.Seconds()))
	}
}

// recordLineSend 记一次明细行投递，口径与快照同（都是阻塞发，等的是同一条队列）。
func (s *progressStats) recordLineSend(wait time.Duration) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.lineSent++
	s.lineSum += wait
	if wait > s.lineMax {
		s.lineMax = wait
	}
	seq := s.lineSent
	s.mu.Unlock()
	if s.el != nil {
		s.el.Debug(fmt.Sprintf("进度投递：第 %d 条明细行交到事件循环手上用了 %.3fs", seq, wait.Seconds()))
	}
}

// observeMsg 事件循环处理了一条消息（Update 入口）。
func (s *progressStats) observeMsg() {
	if s != nil {
		s.msgs.Add(1)
	}
}

// observeView 渲染了一帧，cost 是这帧正文的构建耗时（终端写出在渲染器自己的 ticker 上，
// 不在这段里）。
func (s *progressStats) observeView(cost time.Duration) {
	if s == nil {
		return
	}
	s.frames.Add(1)
	s.viewNanos.Add(int64(cost))
	ns := int64(cost)
	for {
		cur := s.viewMaxNS.Load()
		if ns <= cur || s.viewMaxNS.CompareAndSwap(cur, ns) {
			break
		}
	}
}

// logSummary 收尾小结：一行把两侧合计写出。换环境对比时最该看的就是这一行——
// 正常环境里投递等待应当是个零头，出问题的环境里它会长到秒级。
func (s *progressStats) logSummary() {
	if s == nil || s.el == nil {
		return
	}
	s.mu.Lock()
	snapSent, snapSum, snapMax := s.snapSent, s.snapSum, s.snapMax
	lineSent, lineSum, lineMax := s.lineSent, s.lineSum, s.lineMax
	s.mu.Unlock()
	s.el.Debug(fmt.Sprintf(
		"进度链路小结：快照 %d 份（投递等待合计 %.3fs、最长 %.3fs）；明细行 %d 条（合计 %.3fs、最长 %.3fs）；事件循环 %d 条消息、%d 帧，View 合计 %.3fs（单帧最长 %.3fs）",
		snapSent, snapSum.Seconds(), snapMax.Seconds(),
		lineSent, lineSum.Seconds(), lineMax.Seconds(),
		s.msgs.Load(), s.frames.Load(),
		time.Duration(s.viewNanos.Load()).Seconds(), time.Duration(s.viewMaxNS.Load()).Seconds()))
}
