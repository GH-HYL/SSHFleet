package httpserver

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync/atomic"
	"time"

	"SSHFleet/internal/core"
	"SSHFleet/internal/interrupt"
	"SSHFleet/internal/jsonproc"
	"SSHFleet/internal/log"
	"SSHFleet/internal/ssh"
	"context"
)

// ErrorResponse 错误响应结构
type ErrorResponse struct {
	Success bool `json:"success"`
	Error   struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

var server *http.Server
var requestUsed int32 // 0=可用, 1=已处理

// Start 启动 HTTP Server，处理一次请求后退出
func Start(port int, logPath string) {
	log.InitLogger(logPath)

	interruptHandler := interrupt.NewInterruptHandler()
	interruptHandler.Setup()

	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/execute", handleExecute)

	server = &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: mux,
	}

	go func() {
		<-interruptHandler.Done()
		log.Zlog.Info("收到中断信号，开始退出...")
		server.Shutdown(interruptHandler.Context())
	}()

	log.Zlog.Succ(fmt.Sprintf("HTTP Server 启动，监听端口 %d", port))
	if err := server.ListenAndServe(); err != http.ErrServerClosed {
		log.Zlog.Error(fmt.Sprintf("HTTP Server 异常退出: %v", err))
	}
}

// handleExecute 处理执行请求，完成后关闭服务器
func handleExecute(w http.ResponseWriter, r *http.Request) {
	// 一次性防护：只允许处理一次请求
	if !atomic.CompareAndSwapInt32(&requestUsed, 0, 1) {
		writeError(w, http.StatusServiceUnavailable, "ALREADY_USED", "服务已被调用，仅支持一次请求")
		return
	}

	log.Zlog.Info("收到执行请求，开始处理...")

	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "读取请求体失败")
		go server.Shutdown(context.Background())
		return
	}
	defer r.Body.Close()

	req, err := jsonproc.ParseRequest(body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "MISSING_FIELD", err.Error())
		go server.Shutdown(context.Background())
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	tasks := make([]*core.SSHTask, 0, len(req.Nodes))
	for _, node := range req.Nodes {
		tasks = append(tasks, &core.SSHTask{
			Seq: node.Seq,
			Config: &ssh.SSHConfig{
				IP:             node.IP,
				Port:           node.Port,
				User:           node.User,
				Password:       node.Password,
				ConnectTimeout: time.Duration(req.Options.ConnectTimeout) * time.Second,
				ExecTimeout:    time.Duration(req.Options.ExecTimeout) * time.Second,
			},
			Command: req.Command,
		})
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "不支持流式响应")
		go server.Shutdown(context.Background())
		return
	}

	executor := core.NewBatchExecutor(req.Options.Concurrency, len(tasks), r.Context())
	resultChan := executor.Run(tasks)

	total, success, failed := len(tasks), 0, 0
	for result := range resultChan {
		if err := WriteSSE(w, result); err != nil {
			log.Zlog.Error(fmt.Sprintf("SSE 写入失败: %v", err))
			return
		}
		flusher.Flush()
		if result.ConnectSuccess && result.ExitCode == 0 {
			success++
		} else {
			failed++
		}
	}

	done := ssh.DoneResponse{Type: "done", Total: total, Success: success, Failed: failed}
	WriteSSE(w, done)
	flusher.Flush()

	log.Zlog.Succ(fmt.Sprintf("任务执行完成: total=%d, success=%d, failed=%d", total, success, failed))
	go server.Shutdown(context.Background())
}

func writeError(w http.ResponseWriter, statusCode int, code string, message string) {
	log.Zlog.Error(fmt.Sprintf("请求处理失败: code=%s, message=%s", code, message))
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(ErrorResponse{Success: false, Error: struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}{Code: code, Message: message}})
}
