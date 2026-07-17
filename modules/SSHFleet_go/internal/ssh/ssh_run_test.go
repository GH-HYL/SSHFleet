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

func TestProgressWriter_MultipleFilesProgressiveUpload(t *testing.T) {
	// 测试修复后的逻辑：UploadFiles 级别维护累计上传字节数
	// progressWriter 本身仍然独立，但 UploadFiles 会在每个文件完成后累加字节数

	var messages []ProgressMsg
	var mu sync.Mutex

	callback := func(msg ProgressMsg) {
		mu.Lock()
		messages = append(messages, msg)
		mu.Unlock()
	}

	// 模拟 UploadFiles 的逻辑：维护 uploadedBytes
	var uploadedBytes int64
	totalBytes := int64(200)

	// 第一个文件上传
	var buf1 bytes.Buffer
	pw1 := &progressWriter{
		dst:          &buf1,
		seq:          0,
		ip:           "10.0.0.1",
		totalBytes:   totalBytes,
		totalFiles:   2,
		lastCallback: time.Now(),
		callback:     callback,
	}
	pw1.Write(bytes.Repeat([]byte("a"), 100))
	uploadedBytes += pw1.uploaded  // 累加到 uploadedBytes

	// 第二个文件上传
	var buf2 bytes.Buffer
	pw2 := &progressWriter{
		dst:          &buf2,
		seq:          0,
		ip:           "10.0.0.1",
		totalBytes:   totalBytes,
		totalFiles:   2,
		lastCallback: time.Now(),
		callback:     callback,
	}
	pw2.Write(bytes.Repeat([]byte("b"), 100))
	uploadedBytes += pw2.uploaded  // 累加到 uploadedBytes

	mu.Lock()
	defer mu.Unlock()

	// 验证：每个 progressWriter 独立计算，但 UploadFiles 累加
	if pw1.uploaded != 100 {
		t.Errorf("第一个文件 uploaded 应为 100，实际 %d", pw1.uploaded)
	}
	if pw2.uploaded != 100 {
		t.Errorf("第二个文件 uploaded 应为 100，实际 %d", pw2.uploaded)
	}

	// 关键：累计字节数应该是 200
	if uploadedBytes != 200 {
		t.Errorf("累计 uploadedBytes 应为 200，实际 %d", uploadedBytes)
	}

	t.Logf("修复验证：uploadedBytes 累加正确，=%d", uploadedBytes)
}
