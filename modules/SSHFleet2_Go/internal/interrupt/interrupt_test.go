package interrupt

import (
	"context"
	"os"
	"testing"
)

// 启动段收到信号：交回塔尖收场一次，同时上下文已取消。
func TestEarlySignalCleansUpOnce(t *testing.T) {
	cleaned := 0
	h := start(func() { cleaned++ }, make(chan os.Signal, 1))

	h.react()
	h.react() // 第一下就退了，第二下只是把「按两次」这件事照原样喂进来

	if cleaned != 1 {
		t.Fatalf("启动段收场次数 want 1，got %d", cleaned)
	}
	if err := h.Context().Err(); err != context.Canceled {
		t.Fatalf("上下文 want context.Canceled，got %v", err)
	}
}

// 执行段连按：只取消，一次都不收场——「别多中断几次就退出了」。
func TestRunningSignalOnlyCancels(t *testing.T) {
	cleaned := 0
	h := start(func() { cleaned++ }, make(chan os.Signal, 1))
	h.Handoff()

	for i := 0; i < 5; i++ {
		h.react()
	}

	if cleaned != 0 {
		t.Fatalf("执行段不该收场，got %d 次", cleaned)
	}
	if err := h.Context().Err(); err != context.Canceled {
		t.Fatalf("上下文 want context.Canceled，got %v", err)
	}
}

// 上下文是同一个：翻位前后拿到的是同一个取消通知，执行站不必换一个。
func TestContextSurvivesHandoff(t *testing.T) {
	h := start(func() {}, make(chan os.Signal, 1))
	before := h.Context()
	h.Handoff()
	if h.Context() != before {
		t.Fatal("翻位之后换了上下文，执行站会拿到另一个")
	}
}
