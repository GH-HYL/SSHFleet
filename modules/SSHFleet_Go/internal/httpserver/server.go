package httpserver

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"SSHFleet/internal/core"
	"SSHFleet/internal/interrupt"
	"SSHFleet/internal/jsonproc"
	"SSHFleet/internal/localfs"
	"SSHFleet/internal/log"
	"SSHFleet/internal/ssh"
	"context"

	"go.uber.org/zap"
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
var processKey string  // 进程认证key

// Start 启动 HTTP Server，处理一次请求后退出
func Start(port int, logPath string, key string) error {
	processKey = key
	if err := log.InitLogger(logPath); err != nil {
		return err
	}

	log.Zlog.Info("服务器配置", zap.Int("port", port), zap.String("logPath", logPath))

	shutdownSignal = make(chan struct{})

	interruptHandler := interrupt.NewInterruptHandler()
	interruptHandler.Setup()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/health", handleHealth)
	mux.HandleFunc("POST /api/v1/execute", validateKey(handleExecute))
	mux.HandleFunc("POST /api/v1/upload", validateKey(handleUpload))
	mux.HandleFunc("POST /api/v1/download", validateKey(handleDownload))
	mux.HandleFunc("POST /api/v1/shutdown", validateKey(handleShutdown))

	server = &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: mux,
	}

	go func() {
		<-interruptHandler.Done()
		log.Zlog.Info("收到中断信号，开始退出...")
		server.Shutdown(interruptHandler.Context())
	}()

	log.Zlog.Succ("HTTP Server 启动", zap.Int("port", port))

	if err := server.ListenAndServe(); err != http.ErrServerClosed {
		log.Zlog.Error("HTTP Server 异常退出", zap.Error(err))
	}
	return nil
}

// handleExecute 处理执行请求，完成后关闭服务器
func handleExecute(w http.ResponseWriter, r *http.Request) {
	// 一次性防护：只允许处理一次请求
	if !atomic.CompareAndSwapInt32(&requestUsed, 0, 1) {
		log.Zlog.Warn("请求被拒绝: 服务已被调用，仅支持一次请求", zap.String("path", r.URL.Path))
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

	log.Zlog.Info("执行请求解析成功",
		zap.String("command", req.Command),
		zap.Int("nodes", len(req.Nodes)),
		zap.Int("concurrency", req.Options.Concurrency),
		zap.Int("connectTimeout", req.Options.ConnectTimeout),
		zap.Int("execTimeout", req.Options.ExecTimeout))

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

	// 使用独立的 context，不依赖 HTTP 请求（防止客户端断开导致所有任务终止）
	execCtx, execCancel := context.WithCancel(context.Background())
	defer execCancel()

	executor := core.NewBatchExecutor(req.Options.Concurrency, len(tasks), execCtx)
	resultChan := executor.Run(tasks)

	total, success, failed := len(tasks), 0, 0
	connSuccess, connFailed := 0, 0
	for result := range resultChan {
		if err := WriteSSE(w, result); err != nil {
			log.Zlog.Error("SSE 写入失败", zap.Error(err))
			return
		}
		flusher.Flush()
		if result.ConnectSuccess {
			connSuccess++
		} else {
			connFailed++
		}
		if result.ConnectSuccess && result.ExitCode != nil && *result.ExitCode == 0 {
			success++
		} else {
			failed++
		}
	}

	log.Zlog.Info("连接统计", zap.Int("total", total), zap.Int("connSuccess", connSuccess), zap.Int("connFailed", connFailed))

	done := ssh.DoneResponse{Type: "done", Total: total, Success: success, Failed: failed}
	WriteSSE(w, done)
	flusher.Flush()

	log.Zlog.Succ("任务执行完成", zap.Int("total", total), zap.Int("success", success), zap.Int("failed", failed))

	// 等待客户端发送关闭信号，2分钟超时防御
	log.Zlog.Info("等待客户端发送关闭信号...")
	select {
	case <-shutdownSignal:
		// handleShutdown 已记录日志
	case <-time.After(2 * time.Minute):
		log.Zlog.Warn("等待关闭信号超时(2分钟)，强制退出")
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
		log.Zlog.Warn("请求被拒绝: 服务已被调用，仅支持一次请求", zap.String("path", r.URL.Path))
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

	req, err := jsonproc.ParseUploadRequest(body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "MISSING_FIELD", err.Error())
		go server.Shutdown(context.Background())
		return
	}
	log.Zlog.Info("上传请求解析成功",
		zap.String("filePath", req.FilePath),
		zap.String("remotePath", req.RemotePath),
		zap.Int("nodes", len(req.Nodes)),
		zap.Bool("sudo", req.Options.Sudo))

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
	log.Zlog.Info("文件清单收集完成", zap.Int("count", len(fileItems)))

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

	// 计算每个节点的总字节数
	var totalBytesPerNode int64
	for _, item := range fileItems {
		totalBytesPerNode += item.FileSize
	}

	// 发送 init 消息
	initMsg := map[string]interface{}{
		"type":               "init",
		"total_nodes":        len(req.Nodes),
		"total_bytes_per_node": totalBytesPerNode,
	}
	WriteSSE(w, initMsg)
	flusher.Flush()

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

	// 创建 progress channel 并启动消费协程
	progressChan := make(chan ssh.ProgressMsg, len(tasks)*10)

	// 使用 mutex 保护 SSE 写入，防止并发写入导致分块编码错误
	var sseMu sync.Mutex
	writeSSESafe := func(data interface{}) error {
		sseMu.Lock()
		defer sseMu.Unlock()
		if err := WriteSSE(w, data); err != nil {
			return err
		}
		flusher.Flush()
		return nil
	}

	var progressWg sync.WaitGroup
	progressWg.Add(1)
	go func() {
		defer progressWg.Done()
		for msg := range progressChan {
			if err := writeSSESafe(msg); err != nil {
				log.Zlog.Error("SSE progress 写入失败", zap.Error(err))
				return
			}
		}
	}()

	// 使用独立的 context，不依赖 HTTP 请求（防止客户端断开导致所有任务终止）
	uploadCtx, uploadCancel := context.WithCancel(context.Background())
	defer uploadCancel()

	// 执行上传
	executor := core.NewBatchUploadExecutor(req.Options.Concurrency, len(tasks), uploadCtx, progressChan)
	resultChan := executor.Run(tasks)

	total, success, failed := len(tasks), 0, 0
	connSuccess, connFailed := 0, 0
	for result := range resultChan {
		if err := writeSSESafe(result); err != nil {
			log.Zlog.Error("SSE 写入失败", zap.Error(err))
			close(progressChan)
			return
		}
		if result.ConnectSuccess {
			connSuccess++
		} else {
			connFailed++
		}
		if result.ConnectSuccess && result.Error == nil && result.ExitCode != nil && *result.ExitCode == 0 {
			success++
		} else {
			failed++
		}
	}

	// 关闭 progress channel 并等待消费完毕
	close(progressChan)
	progressWg.Wait()

	log.Zlog.Info("连接统计", zap.Int("total", total), zap.Int("connSuccess", connSuccess), zap.Int("connFailed", connFailed))

	done := ssh.DoneResponse{Type: "done", Total: total, Success: success, Failed: failed}
	WriteSSE(w, done)
	flusher.Flush()

	log.Zlog.Succ("上传任务完成", zap.Int("total", total), zap.Int("success", success), zap.Int("failed", failed))

	// 等待关闭信号，2分钟超时防御
	log.Zlog.Info("等待客户端发送关闭信号...")
	select {
	case <-shutdownSignal:
	case <-time.After(2 * time.Minute):
		log.Zlog.Warn("等待关闭信号超时(2分钟)，强制退出")
	}
	go server.Shutdown(context.Background())
}

// handleDownload 处理下载请求
func handleDownload(w http.ResponseWriter, r *http.Request) {
	// 一次性防护
	if !atomic.CompareAndSwapInt32(&requestUsed, 0, 1) {
		log.Zlog.Warn("请求被拒绝: 服务已被调用，仅支持一次请求", zap.String("path", r.URL.Path))
		writeError(w, http.StatusServiceUnavailable, "ALREADY_USED", "服务已被调用，仅支持一次请求")
		return
	}

	log.Zlog.Info("收到下载请求，开始处理...")

	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "读取请求体失败")
		go server.Shutdown(context.Background())
		return
	}
	defer r.Body.Close()

	req, err := jsonproc.ParseDownloadRequest(body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "MISSING_FIELD", err.Error())
		go server.Shutdown(context.Background())
		return
	}
	log.Zlog.Info("下载请求解析成功",
		zap.String("remotePath", req.RemotePath),
		zap.String("localPath", req.LocalPath),
		zap.Int("nodes", len(req.Nodes)),
		zap.Bool("sudo", req.Options.Sudo))

	// 将 local_path 转为绝对路径
	absLocalPath, err := filepath.Abs(req.LocalPath)
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_PATH", fmt.Sprintf("local_path 路径解析失败: %s", req.LocalPath))
		go server.Shutdown(context.Background())
		return
	}
	req.LocalPath = absLocalPath

	// 校验本地 local_path 是否存在且是目录
	if info, err := os.Stat(req.LocalPath); err != nil || !info.IsDir() {
		writeError(w, http.StatusBadRequest, "INVALID_PATH", fmt.Sprintf("local_path 不存在或不是目录: %s", req.LocalPath))
		go server.Shutdown(context.Background())
		return
	}

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

	// 构建下载任务
	tasks := make([]*core.DownloadTask, 0, len(req.Nodes))
	for _, node := range req.Nodes {
		tasks = append(tasks, &core.DownloadTask{
			Seq: node.Seq,
			Config: &ssh.SSHConfig{
				IP:             node.IP,
				Port:           node.Port,
				User:           node.User,
				Password:       node.Password,
				ConnectTimeout: time.Duration(req.Options.ConnectTimeout) * time.Second,
				ExecTimeout:    time.Duration(req.Options.ExecTimeout) * time.Second,
			},
			RemotePath: req.RemotePath,
			LocalPath:  req.LocalPath,
			UseSudo:    req.Options.Sudo,
		})
	}

	// 创建 progress channel 并启动消费协程
	progressChan := make(chan ssh.ProgressMsg, len(tasks)*10)

	// 使用 mutex 保护 SSE 写入
	var sseMu sync.Mutex
	writeSSESafe := func(data interface{}) error {
		sseMu.Lock()
		defer sseMu.Unlock()
		if err := WriteSSE(w, data); err != nil {
			return err
		}
		flusher.Flush()
		return nil
	}

	var progressWg sync.WaitGroup
	progressWg.Add(1)
	go func() {
		defer progressWg.Done()
		for msg := range progressChan {
			if err := writeSSESafe(msg); err != nil {
				log.Zlog.Error("SSE progress 写入失败", zap.Error(err))
				return
			}
		}
	}()

	// 使用独立的 context
	downloadCtx, downloadCancel := context.WithCancel(context.Background())
	defer downloadCancel()

	// 执行下载
	executor := core.NewBatchDownloadExecutor(req.Options.Concurrency, len(tasks), downloadCtx, progressChan)
	resultChan := executor.Run(tasks)

	total, success, failed := len(tasks), 0, 0
	connSuccess, connFailed := 0, 0
	for result := range resultChan {
		if err := writeSSESafe(result); err != nil {
			log.Zlog.Error("SSE 写入失败", zap.Error(err))
			close(progressChan)
			return
		}
		if result.ConnectSuccess {
			connSuccess++
		} else {
			connFailed++
		}
		if result.ConnectSuccess && result.Error == nil && result.ExitCode != nil && *result.ExitCode == 0 {
			success++
		} else {
			failed++
		}
	}

	// 关闭 progress channel 并等待消费完毕
	close(progressChan)
	progressWg.Wait()

	log.Zlog.Info("连接统计", zap.Int("total", total), zap.Int("connSuccess", connSuccess), zap.Int("connFailed", connFailed))

	done := ssh.DoneResponse{Type: "done", Total: total, Success: success, Failed: failed}
	WriteSSE(w, done)
	flusher.Flush()

	log.Zlog.Succ("下载任务完成", zap.Int("total", total), zap.Int("success", success), zap.Int("failed", failed))

	// 等待关闭信号
	log.Zlog.Info("等待客户端发送关闭信号...")
	select {
	case <-shutdownSignal:
	case <-time.After(2 * time.Minute):
		log.Zlog.Warn("等待关闭信号超时(2分钟)，强制退出")
	}
	go server.Shutdown(context.Background())
}

// handleHealth 健康检查端点，无需认证
func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func validateKey(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		key := r.Header.Get("X-SSH-Fleet-Key")
		if key != processKey {
			writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "无效的认证key")
			return
		}
		next(w, r)
	}
}

func writeError(w http.ResponseWriter, statusCode int, code string, message string) {
	log.Zlog.Error("请求处理失败", zap.String("code", code), zap.String("message", message))
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(ErrorResponse{Success: false, Error: struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}{Code: code, Message: message}})
}
