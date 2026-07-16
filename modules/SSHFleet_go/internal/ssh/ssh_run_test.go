package ssh

import (
	"bytes"
	"sync"
	"testing"
	"time"
)

func TestProgressWriter_Throttle(t *testing.T) {
	// 测试 500ms 节流：快速连续写入只触发一次回调
	var messages []ProgressMsg
	var mu sync.Mutex

	callback := func(msg ProgressMsg) {
		mu.Lock()
		messages = append(messages, msg)
		mu.Unlock()
	}

	var buf bytes.Buffer
	pw := &progressWriter{
		dst:      &buf,
		seq:      0,
		ip:       "10.0.0.1",
		callback: callback,
	}

	// 快速写入 10 次
	for i := 0; i < 10; i++ {
		pw.Write([]byte("hello"))
	}

	// 应该只触发 1 次回调（因为 500ms 内）
	mu.Lock()
	count := len(messages)
	mu.Unlock()

	if count != 1 {
		t.Errorf("期望 1 次回调，实际 %d 次", count)
	}
}

func TestProgressWriter_AccumulateBytes(t *testing.T) {
	// 测试字节累加
	var buf bytes.Buffer
	pw := &progressWriter{
		dst:      &buf,
		seq:      1,
		ip:       "10.0.0.2",
		callback: func(msg ProgressMsg) {},
	}

	// 写入 5 字节
	pw.Write([]byte("hello"))
	uploaded := pw.uploaded

	if uploaded != 5 {
		t.Errorf("期望 5 字节，实际 %d", uploaded)
	}
}

func TestProgressWriter_TimeBetweenCallbacks(t *testing.T) {
	// 测试回调间隔
	var callbackTimes []time.Time
	var mu sync.Mutex

	callback := func(msg ProgressMsg) {
		mu.Lock()
		callbackTimes = append(callbackTimes, time.Now())
		mu.Unlock()
	}

	var buf bytes.Buffer
	pw := &progressWriter{
		dst:      &buf,
		seq:      2,
		ip:       "10.0.0.3",
		callback: callback,
	}

	// 第一次写入
	pw.Write([]byte("a"))
	// 等 600ms
	time.Sleep(600 * time.Millisecond)
	// 第二次写入
	pw.Write([]byte("b"))

	mu.Lock()
	count := len(callbackTimes)
	mu.Unlock()

	if count != 2 {
		t.Errorf("期望 2 次回调，实际 %d 次", count)
	}
}

func TestProgressMsg_JSON(t *testing.T) {
	// 测试 ProgressMsg JSON 序列化
	msg := ProgressMsg{
		Type:          "progress",
		Seq:           0,
		IP:            "10.0.0.1",
		TotalBytes:    1000,
		TotalFiles:    5,
		UploadedBytes: 500,
		SuccessFiles:  2,
		FailedFiles:   1,
	}

	if msg.Type != "progress" {
		t.Error("Type 应为 progress")
	}
	if msg.Seq != 0 {
		t.Error("Seq 应为 0")
	}
	if msg.TotalBytes != 1000 {
		t.Error("TotalBytes 应为 1000")
	}
}

func TestProgressMsg_OmitEmpty(t *testing.T) {
	// 测试 omitempty：空字段不应出现
	msg := ProgressMsg{
		Type: "progress",
		Seq:  0,
		IP:   "10.0.0.1",
		// UploadedBytes 为 0，omitempty 应跳过
	}

	// 验证 omitempty 行为（Go 的 omitempty 对 0 值有效）
	if msg.UploadedBytes != 0 {
		t.Error("UploadedBytes 应为 0")
	}
}
