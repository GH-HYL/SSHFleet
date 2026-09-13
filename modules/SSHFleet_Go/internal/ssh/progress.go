package ssh

import (
	"io"
	"sync"
	"time"
)

// progressThrottle 进度回调节流间隔（对位旧实现 500ms）。
const progressThrottle = 500 * time.Millisecond

// progressWriter 带进度回调的写入器（上传路径；满足 io.Writer，spec D32 条件 3）。
// 与旧实现唯一差异：callback 判空（旧 progressWriter.Write 未判空，callback 为 nil 会 panic）。
type progressWriter struct {
	dst          io.Writer
	uploaded     int64
	lastCallback time.Time
	seq          int
	ip           string
	totalBytes   int64
	totalFiles   int
	callback     func(Progress)
	mu           sync.Mutex
}

func (pw *progressWriter) Write(p []byte) (int, error) {
	n, err := pw.dst.Write(p)
	pw.mu.Lock()
	pw.uploaded += int64(n)
	if pw.callback != nil && time.Since(pw.lastCallback) >= progressThrottle {
		pw.lastCallback = time.Now()
		pw.callback(Progress{
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

// progressReader 带进度回调的读取器（下载路径；满足 io.Reader）。
type progressReader struct {
	src          io.Reader
	downloaded   int64
	lastCallback time.Time
	seq          int
	ip           string
	totalBytes   int64
	totalFiles   int
	callback     func(Progress)
	mu           sync.Mutex
}

func (pr *progressReader) Read(p []byte) (int, error) {
	n, err := pr.src.Read(p)
	pr.mu.Lock()
	pr.downloaded += int64(n)
	shouldCallback := time.Since(pr.lastCallback) >= progressThrottle
	if err == io.EOF {
		shouldCallback = true
	}
	if shouldCallback && pr.callback != nil {
		pr.lastCallback = time.Now()
		pr.callback(Progress{
			Seq:             pr.seq,
			IP:              pr.ip,
			DownloadedBytes: pr.downloaded,
			TotalBytes:      pr.totalBytes,
			TotalFiles:      pr.totalFiles,
		})
	}
	pr.mu.Unlock()
	return n, err
}

// progressPacer 循环级逐文件进度节流器（与字节级读写器同口径；零值可用）。
type progressPacer struct{ last time.Time }

func (p *progressPacer) allow() bool {
	now := time.Now()
	if now.Sub(p.last) >= progressThrottle {
		p.last = now
		return true
	}
	return false
}
