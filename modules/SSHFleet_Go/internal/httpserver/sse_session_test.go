package httpserver

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"SSHFleet/internal/ssh"
)

// failingSSEWriter 模拟客户端断开：所有 Write 调用都返回错误（底层连接已关闭）
type failingSSEWriter struct {
	header   http.Header
	writeErr error
}

func (f *failingSSEWriter) Header() http.Header {
	if f.header == nil {
		f.header = http.Header{}
	}
	return f.header
}

func (f *failingSSEWriter) WriteHeader(code int) {}
func (f *failingSSEWriter) Write(p []byte) (int, error) {
	return 0, f.writeErr
}
func (f *failingSSEWriter) Flush() {}

// TestSseSessionWriteSerializesSSEFrame 单条写入：序列化 + data 帧格式 + Flush 后可见
func TestSseSessionWriteSerializesSSEFrame(t *testing.T) {
	rec := httptest.NewRecorder()
	s := NewSseSession(rec)

	payload := map[string]interface{}{"type": "done", "total": 3}
	if err := s.Write(payload); err != nil {
		t.Fatalf("Write 返回错误: %v", err)
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	want := fmt.Sprintf("data: %s\n\n", jsonData)
	if got := rec.Body.String(); got != want {
		t.Errorf("SSE 帧不匹配\n got: %q\nwant: %q", got, want)
	}
}

// TestSseSessionWriteConcurrentFramesIntact 并发写入：帧互不交错、一条不丢
func TestSseSessionWriteConcurrentFramesIntact(t *testing.T) {
	rec := httptest.NewRecorder()
	s := NewSseSession(rec)

	const n = 50
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(seq int) {
			defer wg.Done()
			if err := s.Write(map[string]interface{}{"seq": seq}); err != nil {
				t.Errorf("Write(seq=%d) 返回错误: %v", seq, err)
			}
		}(i)
	}
	wg.Wait()

	// 按 \n\n 分隔，每段应是一个完整 data 帧
	frames := regexp.MustCompile(`data: (.*)\n\n`).FindAllStringSubmatch(rec.Body.String(), -1)
	if len(frames) != n {
		t.Fatalf("帧数不匹配: got %d, want %d\nbody: %q", len(frames), n, rec.Body.String())
	}

	seen := make(map[int]bool, n)
	for _, m := range frames {
		var v struct {
			Seq int `json:"seq"`
		}
		if err := json.Unmarshal([]byte(m[1]), &v); err != nil {
			t.Errorf("帧不是合法 JSON: %q (%v)", m[1], err)
			continue
		}
		seen[v.Seq] = true
	}
	for i := 0; i < n; i++ {
		if !seen[i] {
			t.Errorf("缺少 seq=%d 的帧（写入交错或丢失）", i)
		}
	}
}

// TestSseSessionWritePropagatesError 底层写入失败时错误向上传播
func TestSseSessionWritePropagatesError(t *testing.T) {
	w := &failingSSEWriter{writeErr: errors.New("connection closed")}
	s := NewSseSession(w)

	if err := s.Write(map[string]interface{}{"type": "result"}); err == nil {
		t.Fatal("底层连接断开时 Write 应返回错误，实际返回 nil")
	}
}

// parseSSESeqs 提取帧的 seq 序列（供进度顺序断言）
func parseSSESeqs(t *testing.T, body string) []int {
	t.Helper()
	frames := regexp.MustCompile(`data: (.*)\n\n`).FindAllStringSubmatch(body, -1)
	seqs := make([]int, 0, len(frames))
	for _, m := range frames {
		var v struct {
			Seq int `json:"seq"`
		}
		if err := json.Unmarshal([]byte(m[1]), &v); err != nil {
			t.Fatalf("帧不是合法 JSON: %q (%v)", m[1], err)
		}
		seqs = append(seqs, v.Seq)
	}
	return seqs
}

// TestSseSessionProgressLifecycle 进度生命周期：消费、顺序、正常关闭
func TestSseSessionProgressLifecycle(t *testing.T) {
	rec := httptest.NewRecorder()
	s := NewSseSession(rec)

	s.EnableProgress(10)
	if s.Progress() == nil {
		t.Fatal("EnableProgress 后 Progress() 不应为 nil")
	}

	const n = 5
	for i := 0; i < n; i++ {
		s.Progress() <- ssh.ProgressMsg{Type: "progress", Seq: i, UploadedBytes: int64(i * 100)}
	}
	s.CloseProgress() // 正常结束路径：关闭通道 → 等消费协程退出

	seqs := parseSSESeqs(t, rec.Body.String())
	if len(seqs) != n {
		t.Fatalf("progress 帧数不匹配: got %d, want %d\nbody: %q", len(seqs), n, rec.Body.String())
	}
	for i, seq := range seqs {
		if seq != i {
			t.Errorf("progress 顺序错乱: index=%d seq=%d", i, seq)
		}
	}
}

// TestSseSessionCloseProgressWithoutEnable 未启用进度时关闭安全
func TestSseSessionCloseProgressWithoutEnable(t *testing.T) {
	rec := httptest.NewRecorder()
	s := NewSseSession(rec)
	s.CloseProgress() // 不应 panic
}

// TestSseSessionConsumerFailureCancelsExec
// 进度消费协程写失败（客户端断开）时必须触发执行取消，
// 否则 worker 会继续向满通道发送并永久阻塞（服务挂死）。
func TestSseSessionConsumerFailureCancelsExec(t *testing.T) {
	w := &failingSSEWriter{writeErr: errors.New("connection closed")}
	s := NewSseSession(w)

	var cancelled atomic.Bool
	s.SetCancel(func() { cancelled.Store(true) })
	s.EnableProgress(10)

	s.Progress() <- ssh.ProgressMsg{Type: "progress", Seq: 1}

	deadline := time.Now().Add(2 * time.Second)
	for !cancelled.Load() && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if !cancelled.Load() {
		t.Error("进度消费写失败后未触发执行取消")
	}

	s.CloseProgress() // 消费协程已退出，关闭应安全返回
}
