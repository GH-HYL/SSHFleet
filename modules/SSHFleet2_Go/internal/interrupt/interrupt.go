// Package interrupt 是横切叶子包：接管中断信号，把它变成「启动段立刻收场」或「执行段取消通知」。
//
// 只做一件事——登记一次、判段、取消、忽略重复。句子不在这里成形（那是 message），
// 进程也不在这里退（那是塔尖的 finish）。两段的分界由 Handoff 翻一格，不是两次登记：
// 撤登记是唯一能让默认动作（当场杀进程）回来的时刻，所以设计里不设这个动作。
//
// 判据见 ADR-0042。
package interrupt

import (
	"context"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
)

// Handle 是一次运行的中断句柄：取消通知从这里取，两段的分界由 Handoff 翻。
type Handle struct {
	ctx       context.Context
	cancel    context.CancelFunc
	onEarly   func()
	running   atomic.Bool
	cleanedUp atomic.Bool
}

// Begin 登记 `SIGINT` 与 `SIGTERM`，返回句柄。
//
// onEarly 是启动段的收场（塔尖传「还控制台、关日志、退 130」那一处），它自己退出进程。
// 登记一直活到进程结束：中途不 Stop、不关通道、不重登记（ADR-0042 决定一）。
func Begin(onEarly func()) *Handle {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt, syscall.SIGTERM)
	return start(onEarly, ch)
}

// start 把「读到信号怎么办」与「去哪儿登记」分开：测试喂一条假通道就够了。
func start(onEarly func(), ch <-chan os.Signal) *Handle {
	ctx, cancel := context.WithCancel(context.Background())
	h := &Handle{ctx: ctx, cancel: cancel, onEarly: onEarly}
	go func() {
		for range ch {
			h.react()
		}
	}()
	return h
}

// Context 是这次运行的取消通知：启动段就建好，执行站照用同一个，不另造第二个。
func (h *Handle) Context() context.Context { return h.ctx }

// Handoff 把状态位翻到执行段：此后收到的信号只取消，不再收场。
//
// 翻位那一刻不能有空档，也不能有「谁都不管」的瞬间——所以才只翻一格，而不是换一套登记。
func (h *Handle) Handoff() { h.running.Store(true) }

// react 处置一次信号：先取消（幂等，按几次都一样），启动段再交回塔尖收场——只交一次。
//
// 执行段收到的重复信号走到这里就完了：不退出、不加快、不强杀（老版没有强杀档）。
func (h *Handle) react() {
	h.cancel()
	if h.running.Load() {
		return
	}
	// 收场只走一次：连按时第二下起什么都不做（真机上第一下已经退了，这里只是不让
	// 「按两次」在别处变成两次收尾）。
	if h.cleanedUp.CompareAndSwap(false, true) {
		h.onEarly()
	}
}
