package interrupt

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"sshexec_go/internal/log"
)

// InterruptHandler 中断处理器
type InterruptHandler struct {
	signals chan os.Signal
	ctx     context.Context
	cancel  func()
}

// NewInterruptHandler 创建处理器
func NewInterruptHandler() *InterruptHandler {
	ctx, cancel := context.WithCancel(context.Background())
	return &InterruptHandler{
		signals: make(chan os.Signal, 1),
		ctx:     ctx,
		cancel:  cancel,
	}
}

// Setup 注册信号监听
func (h *InterruptHandler) Setup() {
	// 监听中断信号
	signal.Notify(h.signals, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-h.signals // 等待中断信号
		log.Zlog.Info("信号中断处理器 - 收到中断信号，开始优雅退出...")
		h.cancel() // 触发上下文取消
	}()
}

// Context 获取上下文
func (h *InterruptHandler) Context() context.Context {
	return h.ctx
}
