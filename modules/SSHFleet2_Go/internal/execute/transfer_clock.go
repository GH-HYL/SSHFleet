package execute

// 传输的时钟（ADR-0063 决定 6、ADR-0065）：一台一份，每次发起之前按剩余武装。
//
// 为什么不是「整段包一层 WithTimeout」：到点时 ctx 只是变成取消状态，`io.CopyBuffer` 不会
// 醒，没有任何东西去关通道。动作要落在每一次发起之前——起一条协程跑这次调用，本协程等它
// 或等到点；到点就关掉它自己那条通道，再等它真的退出来（不等就去删同一个文件，删的是
// 正在被写的那个）。
//
// 两段预算：传输段取 `timeout_transfer`，收尾段取 `timeout_connect`。「等被解开的调用退出」
// 与清理共用收尾那一段，不另设数字。

import (
	"context"
	"time"
)

// transferClock 一台传输的两段预算。
type transferClock struct {
	ctx      context.Context
	deadline time.Time
	transfer time.Duration

	// fired 传输段的到点动作响过没有：段内只响一次（ADR-0065）。
	fired bool

	// finish 收尾那一段：第一次要用的时候才开始计时。
	finish    time.Duration
	finishEnd time.Time
}

// newTransferClock 一台传输的时钟：起点是入口，不是建连——建连有自己的 timeout_connect。
func newTransferClock(ctx context.Context, transfer, finish time.Duration) *transferClock {
	return &transferClock{ctx: ctx, deadline: time.Now().Add(transfer), transfer: transfer, finish: finish}
}

// armEnd 一次发起的结局。
type armEnd int

const (
	armRan     armEnd = iota // 发起并跑完
	armExpired               // 到点：动作已响，调用已退出来
	armStuck                 // 到点：动作已响，但调用在收尾预算内没退出来
	armSkipped               // 剩余已用完，这一次没有发起
)

// ran 这一次发起跑完了，可以接着往下走。
func (e armEnd) ran() bool { return e == armRan }

// arm 把一次发起武装起来。
//
// 跑完就返回；到点先 `stop()` 解开阻塞（只响一次），再在收尾预算内等它退出来。
func (k *transferClock) arm(stop func(), run func()) armEnd {
	// 0 表示不设预算：配置层不许 0（`execution.timeout_transfer` 至少 1 秒），这条路只留给
	// 直接拼纸的调用方。
	if k.transfer <= 0 {
		run()
		return armRan
	}

	remaining := time.Until(k.deadline)
	if remaining <= 0 {
		return armSkipped
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		run()
	}()

	timer := time.NewTimer(remaining)
	defer timer.Stop()
	select {
	case <-done:
		return armRan
	case <-timer.C:
	}

	k.fire(stop)

	wait := k.finishRemaining()
	if wait <= 0 {
		return armStuck
	}
	waitTimer := time.NewTimer(wait)
	defer waitTimer.Stop()
	select {
	case <-done:
		return armExpired
	case <-waitTimer.C:
		return armStuck
	}
}

// fire 传输段的到点动作：只响一次，响过也解除、没响过也解除（ADR-0065）。
func (k *transferClock) fire(stop func()) {
	if k.fired {
		return
	}
	k.fired = true
	k.finishRemaining() // 收尾那一段从这一刻开始计时
	stop()
}

// finishRemaining 收尾那一段还剩多久；第一次问就开始计时。
func (k *transferClock) finishRemaining() time.Duration {
	if k.finishEnd.IsZero() {
		k.finishEnd = time.Now().Add(k.finish)
	}
	return time.Until(k.finishEnd)
}

// interrupted 到点那一刻，父 ctx 是不是已经被取消——两者互斥，与命令模式同口径。
func (k *transferClock) interrupted() bool { return k.ctx.Err() != nil }
