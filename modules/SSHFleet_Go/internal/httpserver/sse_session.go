package httpserver

import (
	"context"
	"net/http"
	"sync"

	"SSHFleet/internal/log"
	"SSHFleet/internal/ssh"

	"go.uber.org/zap"
)

// SseSession 一次批量操作的 SSE 输出会话。
// 持有写入互斥锁与进度通道，是 SSE 输出的单一所有者：
// 主循环与进度消费协程共用同一写入口，写入并发安全、帧不交错；
// 进度通道的创建、消费、关闭由会话统一管理。
type SseSession struct {
	w        http.ResponseWriter
	mu       sync.Mutex
	progress chan ssh.ProgressMsg
	wg       sync.WaitGroup
	closed   bool
	cancel   context.CancelFunc // 进度消费失败（客户端断开）时触发执行取消
}

// NewSseSession 创建基于给定 ResponseWriter 的 SSE 会话
func NewSseSession(w http.ResponseWriter) *SseSession {
	return &SseSession{w: w}
}

// Write 写入一条 SSE 消息（序列化 + 写 data 行 + Flush），并发安全
func (s *SseSession) Write(data interface{}) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return WriteSSE(s.w, data)
}

// SetCancel 设置执行上下文的取消函数。
// 进度消费协程写失败（客户端断开）时调用，让执行 worker 停止发送。
func (s *SseSession) SetCancel(cancel context.CancelFunc) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cancel = cancel
}

// EnableProgress 启用进度输出：创建进度通道并启动消费协程。
// 消费协程逐条把进度消息写入 SSE（与主循环共用写锁）；
// 写失败（客户端断开）时触发执行取消并退出，避免 worker 向满通道阻塞发送。
func (s *SseSession) EnableProgress(buf int) {
	ch := make(chan ssh.ProgressMsg, buf)
	s.mu.Lock()
	s.progress = ch
	s.mu.Unlock()
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		for msg := range ch {
			if err := s.Write(msg); err != nil {
				log.Zlog.Error("SSE progress 写入失败", zap.Error(err))
				s.mu.Lock()
				cancel := s.cancel
				s.mu.Unlock()
				if cancel != nil {
					cancel()
				}
				return
			}
		}
	}()
}

// Progress 返回进度消息发送端（未启用时返回 nil）
func (s *SseSession) Progress() chan<- ssh.ProgressMsg {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.progress
}

// CloseProgress 关闭进度通道并等待消费协程退出。
// 未启用进度时安全返回；重复调用安全（幂等）。
func (s *SseSession) CloseProgress() {
	s.mu.Lock()
	if s.closed || s.progress == nil {
		s.mu.Unlock()
		return
	}
	s.closed = true
	close(s.progress)
	s.mu.Unlock()
	s.wg.Wait()
}
