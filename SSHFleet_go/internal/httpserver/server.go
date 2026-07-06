package httpserver

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	"SSHFleet/internal/core"
	"SSHFleet/internal/interrupt"
	"SSHFleet/internal/jsonproc"
	"SSHFleet/internal/localfs"
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
var requestUsed int32  // 0=可用, 1=已处理
var shutdownSignal chan struct{} // 通知Go关闭的信号通道

// Start 启动 HTTP Server，处理一次请求后退出
func Start(port int, logPath string) error {
	if err := log.InitLogger(logPath); err != nil {
		return err
	}

	log.Zlog.Info(fmt.Sprintf("服务器配置: 监听端口=%d, 日志路径=%q", port, logPath))

	shutdownSignal = make(chan struct{})

	interruptHandler := interrupt.NewInterruptHandler()
	interruptHandler.Setup()

	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/execute", handleExecute)
	mux.HandleFunc("POST /api/v1/upload", handleUpload)
	mux.HandleFunc("POST /api/v1/shutdown", handleShutdown)

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

	// 防御性策略：1 分钟内无请求则自动退出
	go func() {
		time.Sleep(1 * time.Minute)
		if atomic.LoadInt32(&requestUsed) == 0 {
			log.Zlog.Warn("1 分钟内无请求连接，自动退出")
			server.Shutdown(context.Background())
		}
	}()

	if err := server.ListenAndServe(); err != http.ErrServerClosed {
		log.Zlog.Error(fmt.Sprintf("HTTP Server 异常退出: %v", err))
	}
	return nil
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

	log.Zlog.Info(fmt.Sprintf("请求体: size=%d bytes, preview=%q", len(body), truncateString(string(body))))

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

	// 等待客户端发送关闭信号，10分钟超时防御
	log.Zlog.Info("等待客户端发送关闭信号...")
	select {
	case <-shutdownSignal:
		// handleShutdown 已记录日志
	case <-time.After(10 * time.Minute):
		log.Zlog.Warn("等待关闭信号超时(10分钟)，强制退出")
	}
	go server.Shutdown(context.Background())
}

// handleShutdown 处理客户端关闭请求
func handleShutdown(w http.ResponseWriter, r *http.Request) {
	log.Zlog.Info("收到客户端关闭请求")
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})

	// 发送关闭信号
	select {
	case shutdownSignal <- struct{}{}:
	default:
	}
}

// handleUpload 处理上传请求
func handleUpload(w http.ResponseWriter, r *http.Request) {
	// 一次性防护
	if !atomic.CompareAndSwapInt32(&requestUsed, 0, 1) {
		writeError(w, http.StatusServiceUnavailable, "ALREADY_USED", "服务已被调用，仅支持一次请求")
		return
	}

	log.Zlog.Info("收到上传请求，开始处理...")

	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "读取请求体失败")
		go server.Shutdown(context.Background())
		return
	}
	defer r.Body.Close()

	log.Zlog.Info(fmt.Sprintf("请求体: size=%d bytes, preview=%q", len(body), truncateString(string(body))))

	req, err := jsonproc.ParseUploadRequest(body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "MISSING_FIELD", err.Error())
		go server.Shutdown(context.Background())
		return
	}
	log.Zlog.Info(fmt.Sprintf("上传请求解析成功: file_path=%s, remote_path=%s, nodes=%d, sudo=%v",
		req.FilePath, req.RemotePath, len(req.Nodes), req.Options.Sudo))

	// 将 file_path 转为绝对路径（支持相对路径）
	absPath, err := filepath.Abs(req.FilePath)
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_PATH", fmt.Sprintf("file_path 路径解析失败: %s", req.FilePath))
		go server.Shutdown(context.Background())
		return
	}
	req.FilePath = absPath

	// 校验本地 file_path
	if _, err := os.Stat(req.FilePath); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_PATH", fmt.Sprintf("file_path 不存在或不可读: %s", req.FilePath))
		go server.Shutdown(context.Background())
		return
	}

	// 收集文件清单
	fileItems, err := localfs.CollectFiles(req.FilePath)
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_PATH", err.Error())
		go server.Shutdown(context.Background())
		return
	}
	log.Zlog.Info(fmt.Sprintf("文件清单收集完成: %d 个文件", len(fileItems)))

	// 设置 SSE header
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "不支持流式响应")
		go server.Shutdown(context.Background())
		return
	}

	// 构建上传任务
	tasks := make([]*core.UploadTask, 0, len(req.Nodes))
	for _, node := range req.Nodes {
		tasks = append(tasks, &core.UploadTask{
			Seq: node.Seq,
			Config: &ssh.SSHConfig{
				IP:             node.IP,
				Port:           node.Port,
				User:           node.User,
				Password:       node.Password,
				ConnectTimeout: time.Duration(req.Options.ConnectTimeout) * time.Second,
				ExecTimeout:    time.Duration(req.Options.ExecTimeout) * time.Second,
			},
			FileItems:  fileItems,
			RemotePath: req.RemotePath,
			UseSudo:    req.Options.Sudo,
		})
	}

	// 执行上传
	executor := core.NewBatchUploadExecutor(req.Options.Concurrency, len(tasks), r.Context())
	resultChan := executor.Run(tasks)

	total, success, failed := len(tasks), 0, 0
	for result := range resultChan {
		if err := WriteSSE(w, result); err != nil {
			log.Zlog.Error(fmt.Sprintf("SSE 写入失败: %v", err))
			return
		}
		flusher.Flush()
		if result.ConnectSuccess && result.Error == nil && result.ExitCode == 0 {
			success++
		} else {
			failed++
		}
	}

	done := ssh.DoneResponse{Type: "done", Total: total, Success: success, Failed: failed}
	WriteSSE(w, done)
	flusher.Flush()

	log.Zlog.Succ(fmt.Sprintf("上传任务完成: total=%d, success=%d, failed=%d", total, success, failed))

	// 等待关闭信号
	log.Zlog.Info("等待客户端发送关闭信号...")
	select {
	case <-shutdownSignal:
	case <-time.After(10 * time.Minute):
		log.Zlog.Warn("等待关闭信号超时(10分钟)，强制退出")
	}
	go server.Shutdown(context.Background())
}

const maxPreviewLen = 500

func truncateString(s string) string {
	if len(s) <= maxPreviewLen {
		return s
	}
	return s[:maxPreviewLen] + "...(truncated)"
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
