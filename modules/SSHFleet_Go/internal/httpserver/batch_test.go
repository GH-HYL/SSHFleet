package httpserver

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"SSHFleet/internal/log"
)

// TestMain 初始化全局日志（runBatch 内部依赖 log.Zlog）
func TestMain(m *testing.M) {
	if err := log.InitLogger(""); err != nil {
		panic(err)
	}
	os.Exit(m.Run())
}

// TestRunBatchDownloadSSEFailureDoesNotPanic
// 模拟客户端断开（SSE 底层写入全部失败）：runBatch 应走「取消执行 → 排空结果流 → 关闭进度通道」的
// 优雅降级顺序，进程不 panic、不死锁，且故障路径同样走到关闭信号等待（30 秒超时兜底，服务不挂死）。
func TestRunBatchDownloadSSEFailureDoesNotPanic(t *testing.T) {
	processKey = "test-key"
	requestUsed = 0
	shutdownSignal = make(chan struct{}, 1)

	dir := t.TempDir()
	// 节点指向本机未监听端口（立即 connection refused），worker 快速返回
	body := fmt.Sprintf(
		`{"remote_path":"/etc/hosts","local_path":%q,"nodes":[{"seq":1,"ip":"127.0.0.1","port":1,"user":"root","password":"x"}],"options":{"exec_timeout":10,"connect_timeout":1}}`,
		filepath.ToSlash(dir),
	)

	w := &failingSSEWriter{writeErr: errors.New("connection closed")}
	r := httptest.NewRequest("POST", "/api/v1/download", strings.NewReader(body))
	r.Header.Set("X-SSH-Fleet-Key", "test-key")

	runBatchAndVerifyNoPanic(t, validateKey(runBatch(downloadOp)), w, r)
}

// TestRunBatchExecuteSSEFailureDoesNotPanic execute（无进度）路径同样安全降级
func TestRunBatchExecuteSSEFailureDoesNotPanic(t *testing.T) {
	processKey = "test-key"
	requestUsed = 0
	shutdownSignal = make(chan struct{}, 1)

	body := `{"command":"ls","nodes":[{"seq":1,"ip":"127.0.0.1","port":1,"user":"root","password":"x"}],"options":{"exec_timeout":10,"connect_timeout":1}}`

	w := &failingSSEWriter{writeErr: errors.New("connection closed")}
	r := httptest.NewRequest("POST", "/api/v1/execute", strings.NewReader(body))
	r.Header.Set("X-SSH-Fleet-Key", "test-key")

	runBatchAndVerifyNoPanic(t, validateKey(runBatch(executeOp)), w, r)
}

// runBatchAndVerifyNoPanic 执行一次必然走 SSE 故障路径的请求，验证：
// 不 panic、请求槽被占用（runBatch 真正执行）、故障路径消费了关闭信号（走到关闭等待）
func runBatchAndVerifyNoPanic(t *testing.T, handler http.HandlerFunc, w http.ResponseWriter, r *http.Request) {
	t.Helper()

	// waitForShutdown 内部会 go server.Shutdown(...)；测试无真实 server，给空实例兜底
	server = &http.Server{}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer func() {
			if p := recover(); p != nil {
				t.Errorf("runBatch 故障路径 panic: %v", p)
			}
		}()
		handler(w, r)
	}()

	// 轮询等待 runBatch 真正执行（请求槽被占用），带超时防挂死
	deadline := time.Now().Add(5 * time.Second)
	for atomic.LoadInt32(&requestUsed) == 0 {
		if time.Now().After(deadline) {
			t.Fatal("超时：runBatch 未执行（请求槽未被占用）")
		}
		time.Sleep(10 * time.Millisecond)
	}

	// 释放关闭等待：故障路径的 waitForShutdown 应消费该信号
	shutdownSignal <- struct{}{}
	wg.Wait()

	if len(shutdownSignal) != 0 {
		t.Errorf("故障路径未走到关闭信号等待（信号未被消费），服务可能挂死")
	}
}
