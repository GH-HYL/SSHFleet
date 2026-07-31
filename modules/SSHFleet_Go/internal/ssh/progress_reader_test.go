package ssh

import (
	"bytes"
	"io"
	"sync"
	"testing"
	"time"
)

func TestProgressReader_Read(t *testing.T) {
	data := []byte("hello world")
	src := bytes.NewReader(data)

	var messages []ProgressMsg
	var mu sync.Mutex
	callback := func(msg ProgressMsg) {
		mu.Lock()
		messages = append(messages, msg)
		mu.Unlock()
	}

	pr := &progressReader{
		src:        src,
		seq:        0,
		ip:         "10.0.0.1",
		totalBytes: int64(len(data)),
		totalFiles: 1,
		callback:   callback,
	}

	buf := make([]byte, 1024)
	n, err := pr.Read(buf)
	if err != nil && err != io.EOF {
		t.Fatalf("unexpected error: %v", err)
	}

	if n != len(data) {
		t.Errorf("expected to read %d bytes, got %d", len(data), n)
	}
	if string(buf[:n]) != "hello world" {
		t.Errorf("expected 'hello world', got '%s'", string(buf[:n]))
	}
}

func TestProgressReader_AccumulateBytes(t *testing.T) {
	data := []byte("abcdef")
	src := bytes.NewReader(data)

	var mu sync.Mutex
	var lastMsg ProgressMsg
	callback := func(msg ProgressMsg) {
		mu.Lock()
		lastMsg = msg
		mu.Unlock()
	}

	pr := &progressReader{
		src:        src,
		seq:        1,
		ip:         "10.0.0.2",
		totalBytes: 100,
		totalFiles: 3,
		callback:   callback,
	}

	// Read in chunks
	buf := make([]byte, 3)
	pr.Read(buf)
	pr.Read(buf)
	pr.Read(buf) // EOF

	mu.Lock()
	defer mu.Unlock()

	if lastMsg.DownloadedBytes != 6 {
		t.Errorf("expected DownloadedBytes=6, got %d", lastMsg.DownloadedBytes)
	}
}

func TestProgressReader_Throttle(t *testing.T) {
	// Fast reads within 500ms should only trigger one callback
	data := bytes.Repeat([]byte("x"), 10000)
	src := bytes.NewReader(data)

	var messages []ProgressMsg
	var mu sync.Mutex
	callback := func(msg ProgressMsg) {
		mu.Lock()
		messages = append(messages, msg)
		mu.Unlock()
	}

	pr := &progressReader{
		src:        src,
		seq:        2,
		ip:         "10.0.0.3",
		totalBytes: 10000,
		totalFiles: 1,
		callback:   callback,
	}

	// Read all data quickly
	buf := make([]byte, 1024)
	for {
		_, err := pr.Read(buf)
		if err != nil {
			break
		}
	}

	mu.Lock()
	count := len(messages)
	mu.Unlock()

	// Should have at least 1 callback (initial), but not one per chunk
	if count < 1 {
		t.Errorf("expected at least 1 callback, got %d", count)
	}
	if count > 5 {
		t.Errorf("too many callbacks for throttled reader: %d", count)
	}
}

func TestProgressReader_TimeBetweenCallbacks(t *testing.T) {
	var callbackTimes []time.Time
	var mu sync.Mutex
	callback := func(msg ProgressMsg) {
		mu.Lock()
		callbackTimes = append(callbackTimes, time.Now())
		mu.Unlock()
	}

	// Use a slow reader to simulate real network delay
	src := &slowReader{
		data: []byte("aaaa"),
		delay: 600 * time.Millisecond,
	}

	pr := &progressReader{
		src:        src,
		seq:        3,
		ip:         "10.0.0.4",
		totalBytes: 4,
		totalFiles: 1,
		callback:   callback,
	}

	buf := make([]byte, 2)
	// Two reads, each 600ms apart > throttle interval
	pr.Read(buf)
	pr.Read(buf)

	mu.Lock()
	count := len(callbackTimes)
	mu.Unlock()

	if count < 2 {
		t.Errorf("expected at least 2 callbacks, got %d", count)
	}
}

func TestProgressReader_EmptyData(t *testing.T) {
	src := bytes.NewReader([]byte{})
	var callbackCount int
	callback := func(msg ProgressMsg) {
		callbackCount++
	}

	pr := &progressReader{
		src:        src,
		seq:        4,
		ip:         "10.0.0.5",
		totalBytes: 0,
		totalFiles: 1,
		callback:   callback,
	}

	buf := make([]byte, 10)
	_, err := pr.Read(buf)
	if err != io.EOF {
		t.Errorf("expected io.EOF, got %v", err)
	}
}

func TestProgressReader_FieldsPopulated(t *testing.T) {
	src := bytes.NewReader([]byte("test"))
	var capturedMsg ProgressMsg
	callback := func(msg ProgressMsg) {
		capturedMsg = msg
	}

	pr := &progressReader{
		src:        src,
		seq:        5,
		ip:         "10.0.0.6",
		totalBytes: 100,
		totalFiles: 7,
		callback:   callback,
	}

	buf := make([]byte, 10)
	pr.Read(buf)

	if capturedMsg.Type != "progress" {
		t.Errorf("expected Type='progress', got '%s'", capturedMsg.Type)
	}
	if capturedMsg.Seq != 5 {
		t.Errorf("expected Seq=5, got %d", capturedMsg.Seq)
	}
	if capturedMsg.IP != "10.0.0.6" {
		t.Errorf("expected IP='10.0.0.6', got '%s'", capturedMsg.IP)
	}
	if capturedMsg.TotalBytes != 100 {
		t.Errorf("expected TotalBytes=100, got %d", capturedMsg.TotalBytes)
	}
	if capturedMsg.TotalFiles != 7 {
		t.Errorf("expected TotalFiles=7, got %d", capturedMsg.TotalFiles)
	}
}

// slowReader reads one byte at a time with a delay
type slowReader struct {
	data  []byte
	pos   int
	delay time.Duration
}

func (r *slowReader) Read(p []byte) (int, error) {
	if r.pos >= len(r.data) {
		return 0, io.EOF
	}
	time.Sleep(r.delay)
	p[0] = r.data[r.pos]
	r.pos++
	return 1, nil
}
