//go:build integration

package ssh

// 中断两档（06 的复核项）：已连上被中断 → `abort.canceled`；还没连上就被中断 → `abort.connect_canceled`。
//
// 为什么在集成层做而不真按 Ctrl+C：信号在 Windows 上送达 MSYS 进程不可靠，而这里要验的是
// **context 取消之后结果里留下什么事实**，那正是 Ctrl+C 在程序内部的形态。

import (
	"context"
	"strings"
	"testing"
	"time"
)

// 已连上之后被中断：结果里必须是 abort.canceled，不是 connect_canceled。
func TestAbortAfterConnectIsCanceled(t *testing.T) {
	c := intClient(t)
	// 执行超时给足：这一条要验的是「取消」，别让它先被超时收走
	c.cfg.ExecTimeout = 30 * time.Second

	ctx, cancel := context.WithCancel(context.Background())
	// 到点就取消：命令还在远端跑，此时连接是通的
	go func() {
		time.Sleep(800 * time.Millisecond)
		cancel()
	}()

	res := c.RunCommand(ctx, "sleep 30", "", 0)
	got := deref(res.Error)
	if !res.Canceled {
		t.Fatalf("应记成被取消，实际：%q", got)
	}
	if !strings.Contains(got, "sshfleet.abort.canceled: ") {
		t.Fatalf("已连上被中断应发 abort.canceled，实际：%q", got)
	}
	if strings.Contains(got, "connect_canceled") {
		t.Fatalf("连上了却报成建连被取消：%q", got)
	}
}

// 还没连上就被中断：结果里必须是 abort.connect_canceled。
func TestAbortBeforeConnectIsConnectCanceled(t *testing.T) {
	c := intClient(t)

	// 先取消再连：连接阶段立刻看到取消信号
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	res := c.RunCommand(ctx, "echo hi", "", 0)
	got := deref(res.Error)
	if !res.Canceled {
		t.Fatalf("应记成被取消，实际：%q", got)
	}
	if !strings.Contains(got, "sshfleet.abort.connect_canceled: ") {
		t.Fatalf("还没连上就中断应发 connect_canceled，实际：%q", got)
	}
}
