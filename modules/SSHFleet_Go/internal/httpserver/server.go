package httpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"time"

	"SSHFleet/internal/interrupt"
	"SSHFleet/internal/log"

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
var requestUsed int32            // 0=可用, 1=已处理
var shutdownSignal chan struct{} // 通知Go关闭的信号通道
var processKey string            // 进程认证key

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
	mux.HandleFunc("POST /api/v1/execute", validateKey(runBatch(executeOp)))
	mux.HandleFunc("POST /api/v1/upload", validateKey(runBatch(uploadOp)))
	mux.HandleFunc("POST /api/v1/download", validateKey(runBatch(downloadOp)))
	mux.HandleFunc("POST /api/v1/shutdown", validateKey(handleShutdown))

	server = &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: mux,
	}

	// 无请求超时：1 分钟内无任何连接自动退出（首个连接到来即取消计时器，
	// 防止误杀"执行完成后等待 shutdown 信号"的服务）
	requestTimer := time.AfterFunc(1*time.Minute, func() {
		log.Zlog.Warn("1 分钟内无请求连接，自动退出")
		server.Shutdown(context.Background())
	})
	server.ConnState = func(conn net.Conn, state http.ConnState) {
		if state == http.StateNew {
			requestTimer.Stop()
		}
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
