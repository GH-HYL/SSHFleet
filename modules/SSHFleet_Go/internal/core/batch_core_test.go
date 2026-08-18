package core

import (
	"context"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"SSHFleet/internal/log"
	"SSHFleet/internal/ssh"
)

// TestMain 初始化全局日志（runPool 内部依赖 log.Zlog）
func TestMain(m *testing.M) {
	if err := log.InitLogger(""); err != nil {
		panic(err)
	}
	os.Exit(m.Run())
}

// TestRunPoolCancelUnblocksProgressSend
// 进度通道缓冲满（消费方未及时读取）时，执行上下文取消后 worker 的进度发送应解除阻塞、worker 尽快退出。
// 无取消防护时，发送会永久阻塞在 progressChan <- msg 上，worker 永不退出。
func TestRunPoolCancelUnblocksProgressSend(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	progressChan := make(chan ssh.ProgressMsg, 1) // 缓冲 1，无消费者 → 第二次发送即满

	start := make(chan struct{})
	var entered atomic.Bool
	workerDone := make(chan struct{})
	go func() {
		defer close(workerDone)
		tasks := []*SSHTask{{Seq: 1, Config: &ssh.SSHConfig{IP: "127.0.0.1"}, Command: "ls"}}
		resultChan := runPool(ctx, 1, 1, "测试", "任务数量为0", tasks, progressChan,
			func(id int, task *SSHTask, onProgress func(ssh.ProgressMsg)) *ssh.ExecResult {
				entered.Store(true)
				<-start
				for i := 0; i < 1000000; i++ {
					onProgress(ssh.ProgressMsg{Type: "progress", Seq: i})
				}
				return &ssh.ExecResult{Type: "result", Seq: task.Seq}
			})
		for range resultChan {
		}
	}()

	// 等 worker 进入 work 并阻塞在 start 屏障（带超时防挂死）
	deadline := time.Now().Add(5 * time.Second)
	for !entered.Load() {
		if time.Now().After(deadline) {
			t.Fatal("超时：worker 未进入 work")
		}
	}
	close(start)
	// 让 worker 进入发送循环并阻塞在满通道上
	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case <-workerDone:
		// worker 在取消后解除阻塞并退出 ✓
	case <-time.After(2 * time.Second):
		t.Fatal("ctx 取消后 worker 未退出：进度发送永久阻塞")
	}
}
