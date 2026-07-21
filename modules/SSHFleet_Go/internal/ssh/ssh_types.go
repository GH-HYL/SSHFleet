package ssh

import (
	"bytes"
	"io"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
)

const (
	maxFileRetries   = 2
	retryInterval    = 2 * time.Second
	progressThrottle = 500 * time.Millisecond
)

// SSHClient 封装SSH客户端连接
type SSHClient struct {
	client *ssh.Client
	config *SSHConfig
}

// SSHConfig 存储SSH连接配置
type SSHConfig struct {
	IP             string
	Port           int
	User           string
	Password       string
	ConnectTimeout time.Duration
	ExecTimeout    time.Duration
}

// NewSSHClient 创建SSH客户端实例
func NewSSHClient(config *SSHConfig) *SSHClient {
	return &SSHClient{config: config}
}

// connectResult SSH 连接结果
type connectResult struct {
	costTime float64
	err      error
}

// threadSafeWriter 带锁的写入器，确保并发写入安全且保持顺序
type threadSafeWriter struct {
	buf *bytes.Buffer
	mu  sync.Mutex
}

func (w *threadSafeWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.Write(p)
}

// progressWriter 带进度回调的写入器
type progressWriter struct {
	dst          io.Writer
	uploaded     int64
	lastCallback time.Time
	seq          int
	ip           string
	totalBytes   int64
	totalFiles   int
	callback     func(ProgressMsg)
	mu           sync.Mutex
}

func (pw *progressWriter) Write(p []byte) (int, error) {
	n, err := pw.dst.Write(p)
	pw.mu.Lock()
	pw.uploaded += int64(n)
	if time.Since(pw.lastCallback) >= progressThrottle {
		pw.lastCallback = time.Now()
		pw.callback(ProgressMsg{
			Type:          "progress",
			Seq:           pw.seq,
			IP:            pw.ip,
			UploadedBytes: pw.uploaded,
			TotalBytes:    pw.totalBytes,
			TotalFiles:    pw.totalFiles,
		})
	}
	pw.mu.Unlock()
	return n, err
}

// ProgressMsg SSE 进度消息
type ProgressMsg struct {
	Type          string `json:"type"`
	Seq           int    `json:"seq"`
	IP            string `json:"ip"`
	UploadedBytes int64  `json:"uploaded_bytes,omitempty"`
	TotalBytes    int64  `json:"total_bytes,omitempty"`
	TotalFiles    int    `json:"total_files,omitempty"`
	SuccessFiles  int    `json:"success_files,omitempty"`
	FailedFiles   int    `json:"failed_files,omitempty"`
}
