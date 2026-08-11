package httpserver

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"SSHFleet/internal/log"
)

// resetTestState 重置包级全局状态，保证测试隔离。
// shutdownSignal 预填一个信号：让 handler 末尾的 waitForShutdown 立即返回，
// 避免测试阻塞 30 秒；server 设为未监听的 &http.Server{}，Shutdown 直接返回，不会 panic。
func resetTestState(t *testing.T) {
	t.Helper()
	if err := log.InitLogger(""); err != nil {
		t.Fatalf("初始化测试日志失败: %v", err)
	}
	atomic.StoreInt32(&requestUsed, 0)
	processKey = "test-key"
	shutdownSignal = make(chan struct{}, 1)
	shutdownSignal <- struct{}{}
	server = &http.Server{}
}

func testMux() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/health", handleHealth)
	mux.HandleFunc("POST /api/v1/execute", validateKey(runBatch(executeOp)))
	mux.HandleFunc("POST /api/v1/upload", validateKey(runBatch(uploadOp)))
	mux.HandleFunc("POST /api/v1/download", validateKey(runBatch(downloadOp)))
	mux.HandleFunc("POST /api/v1/shutdown", validateKey(handleShutdown))
	return mux
}

func doPost(t *testing.T, path, body string, withKey bool) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	if withKey {
		req.Header.Set("X-SSH-Fleet-Key", "test-key")
	}
	rec := httptest.NewRecorder()
	testMux().ServeHTTP(rec, req)
	return rec
}

func expectCode(t *testing.T, rec *httptest.ResponseRecorder, want int) {
	t.Helper()
	if rec.Code != want {
		t.Fatalf("状态码 = %d, 期望 %d, body = %s", rec.Code, want, rec.Body.String())
	}
}

func TestHealth(t *testing.T) {
	resetTestState(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	rec := httptest.NewRecorder()
	testMux().ServeHTTP(rec, req)
	expectCode(t, rec, http.StatusOK)
	var m map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
		t.Fatalf("health 响应不是合法 JSON: %v", err)
	}
	if m["status"] != "ok" {
		t.Fatalf("health status = %q, 期望 ok", m["status"])
	}
}

// 三个端点无 key 都必须 401
func TestUnauthorizedWithoutKey(t *testing.T) {
	paths := []string{"/api/v1/execute", "/api/v1/upload", "/api/v1/download"}
	for _, p := range paths {
		resetTestState(t)
		rec := doPost(t, p, `{}`, false)
		expectCode(t, rec, http.StatusUnauthorized)
		if !strings.Contains(rec.Body.String(), "UNAUTHORIZED") {
			t.Fatalf("%s 未授权响应缺少 UNAUTHORIZED 码: %s", p, rec.Body.String())
		}
	}
}

// 三个端点坏 JSON 都必须 400 INVALID_REQUEST
func TestBadJSONAllEndpoints(t *testing.T) {
	paths := []string{"/api/v1/execute", "/api/v1/upload", "/api/v1/download"}
	for _, p := range paths {
		resetTestState(t)
		rec := doPost(t, p, `not-json`, true)
		expectCode(t, rec, http.StatusBadRequest)
		if !strings.Contains(rec.Body.String(), "INVALID_REQUEST") {
			t.Fatalf("%s 坏 JSON 响应缺少 INVALID_REQUEST 码: %s", p, rec.Body.String())
		}
	}
}

// fakeNode 一个不可能连通的假节点：127.0.0.1:1 立即拒绝，
// 用于满足 jsonproc "nodes 不能为空" 校验且不触发真实 SSH 交互。
const fakeNode = `{"seq":1,"ip":"127.0.0.1","port":1,"user":"root","password":"x"}`

// fullOptions jsonproc 要求 exec_timeout 等字段大于 0（带前导逗号，便于拼接）
const fullOptions = `,"options":{"concurrency":1,"connect_timeout":1,"exec_timeout":1}`

// execute 正常流：假节点快速连接失败，SSE 输出结果与 done，验证完整处理链
func TestExecuteWithFakeNode(t *testing.T) {
	resetTestState(t)
	body := `{"command":"ls","options":{"concurrency":1,"connect_timeout":1,"exec_timeout":1},"nodes":[` + fakeNode + `]}`
	rec := doPost(t, "/api/v1/execute", body, true)
	expectCode(t, rec, http.StatusOK)
	out := rec.Body.String()
	if !strings.Contains(out, `"type":"done"`) || !strings.Contains(out, `"total":1`) {
		t.Fatalf("execute 正常流应输出 done SSE: %s", out)
	}
}

// upload 路径校验：相对路径必须 400 INVALID_PATH
func TestUploadRelativePathRejected(t *testing.T) {
	resetTestState(t)
	body := `{"file_path":"rel/path/file.txt","remote_path":"/tmp"` + fullOptions + `,"nodes":[` + fakeNode + `]}`
	rec := doPost(t, "/api/v1/upload", body, true)
	expectCode(t, rec, http.StatusBadRequest)
	if !strings.Contains(rec.Body.String(), "INVALID_PATH") {
		t.Fatalf("upload 相对路径响应缺少 INVALID_PATH: %s", rec.Body.String())
	}
}

// upload 路径校验：绝对路径但文件不存在必须 400 INVALID_PATH
func TestUploadMissingFileRejected(t *testing.T) {
	resetTestState(t)
	body := `{"file_path":"C:\\nonexistent_dir_xyz_123\\f.txt","remote_path":"/tmp"` + fullOptions + `,"nodes":[` + fakeNode + `]}`
	rec := doPost(t, "/api/v1/upload", body, true)
	expectCode(t, rec, http.StatusBadRequest)
	if !strings.Contains(rec.Body.String(), "INVALID_PATH") {
		t.Fatalf("upload 不存在文件响应缺少 INVALID_PATH: %s", rec.Body.String())
	}
}

// download 路径校验：remote_path 非 / 开头必须 400 INVALID_PATH
func TestDownloadRemotePathNotAbsRejected(t *testing.T) {
	resetTestState(t)
	body := `{"remote_path":"tmp/foo","local_path":"C:\\tmp"` + fullOptions + `,"nodes":[` + fakeNode + `]}`
	rec := doPost(t, "/api/v1/download", body, true)
	expectCode(t, rec, http.StatusBadRequest)
	if !strings.Contains(rec.Body.String(), "INVALID_PATH") {
		t.Fatalf("download remote 相对路径响应缺少 INVALID_PATH: %s", rec.Body.String())
	}
}

// download 路径校验：local_path 相对必须 400 INVALID_PATH
func TestDownloadLocalPathNotAbsRejected(t *testing.T) {
	resetTestState(t)
	body := `{"remote_path":"/tmp/foo","local_path":"rel/dir"` + fullOptions + `,"nodes":[` + fakeNode + `]}`
	rec := doPost(t, "/api/v1/download", body, true)
	expectCode(t, rec, http.StatusBadRequest)
	if !strings.Contains(rec.Body.String(), "INVALID_PATH") {
		t.Fatalf("download local 相对路径响应缺少 INVALID_PATH: %s", rec.Body.String())
	}
}

// 单次防护：第一次正常占用后，第二次任何请求必须 503 ALREADY_USED
func TestSecondRequestRejected(t *testing.T) {
	resetTestState(t)
	first := doPost(t, "/api/v1/execute", `{"command":"ls"`+fullOptions+`,"nodes":[`+fakeNode+`]}`, true)
	expectCode(t, first, http.StatusOK)

	second := doPost(t, "/api/v1/execute", `{"command":"pwd"`+fullOptions+`,"nodes":[`+fakeNode+`]}`, true)
	expectCode(t, second, http.StatusServiceUnavailable)
	if !strings.Contains(second.Body.String(), "ALREADY_USED") {
		t.Fatalf("第二次请求响应缺少 ALREADY_USED: %s", second.Body.String())
	}
}
