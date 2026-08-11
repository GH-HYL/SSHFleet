package httpserver

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"SSHFleet/internal/jsonproc"
	"SSHFleet/internal/log"
	"SSHFleet/internal/ssh"

	"go.uber.org/zap"
)

// acquireRequestSlot 一次性防护：只允许处理一次请求
// 返回 false 表示已占用（已写 503 ALREADY_USED 响应），handler 应直接返回
func acquireRequestSlot(w http.ResponseWriter, r *http.Request) bool {
	if !atomic.CompareAndSwapInt32(&requestUsed, 0, 1) {
		log.Zlog.Warn("请求被拒绝: 服务已被调用，仅支持一次请求", zap.String("path", r.URL.Path))
		writeError(w, http.StatusServiceUnavailable, "ALREADY_USED", "服务已被调用，仅支持一次请求")
		return false
	}
	return true
}

// readBody 读取请求体
// 返回 false 表示读取失败（已写 400 INVALID_REQUEST 响应并触发关闭），handler 应直接返回
func readBody(w http.ResponseWriter, r *http.Request) ([]byte, bool) {
	body, err := io.ReadAll(r.Body)
	defer r.Body.Close()
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "读取请求体失败")
		go server.Shutdown(context.Background())
		return nil, false
	}
	return body, true
}

// writeParseError 解析/校验错误统一出口（决策 H）
// 通过 errors.As 提取 jsonproc.APIError 的错误码（JSON 语法错→INVALID_REQUEST / 字段缺失→MISSING_FIELD），
// message 按「位置描述 + 原文」包装：<接口> 请求解析失败: <原文>
func writeParseError(w http.ResponseWriter, api string, err error) {
	code := "INVALID_REQUEST"
	var apiErr *jsonproc.APIError
	if errors.As(err, &apiErr) {
		code = apiErr.Code
	}
	writeError(w, http.StatusBadRequest, code, fmt.Sprintf("%s 请求解析失败: %s", api, err.Error()))
	go server.Shutdown(context.Background())
}

// setupSSE 设置 SSE 响应（决策 C1：先 Flusher 检查、后设 SSE header）
// 返回 false 表示不支持流式（已写 500 INTERNAL_ERROR 响应并触发关闭），handler 应直接返回
func setupSSE(w http.ResponseWriter) (http.Flusher, bool) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "不支持流式响应")
		go server.Shutdown(context.Background())
		return nil, false
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	return flusher, true
}

// startProgressConsumer 启动 progress 消费协程，返回等待组（供后续 Wait）
// writeSSE 为调用方提供的加锁写入回调
func startProgressConsumer(progressChan <-chan ssh.ProgressMsg, writeSSE func(interface{}) error) *sync.WaitGroup {
	var progressWg sync.WaitGroup
	progressWg.Add(1)
	go func() {
		defer progressWg.Done()
		for msg := range progressChan {
			if err := writeSSE(msg); err != nil {
				log.Zlog.Error("SSE progress 写入失败", zap.Error(err))
				return
			}
		}
	}()
	return &progressWg
}

// waitForShutdown 等待客户端发送关闭信号，2分钟超时防御
func waitForShutdown() {
	log.Zlog.Info("等待客户端发送关闭信号...")
	select {
	case <-shutdownSignal:
		// handleShutdown 已记录日志
	case <-time.After(2 * time.Minute):
		log.Zlog.Warn("等待关闭信号超时(2分钟)，强制退出")
	}
	go server.Shutdown(context.Background())
}
