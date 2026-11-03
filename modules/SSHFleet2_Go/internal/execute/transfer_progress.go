package execute

// 传输类的进度：节点侧的回调节流、跨节点的聚合，以及那块逐节点的面板。
//
// 三层各管一件事（与老版同一套分工）：
//   - 字节／文件级回调（progressWriter／progressReader／pacer）只管「把数报上去」，500ms 节流；
//   - 聚合器把各节点的数汇总成一份快照，200ms 节流（节点完成时立即渲染一帧）；
//   - 面板只管「怎么画」。

import (
	"io"
	"sort"
	"sync"
	"time"

	"sshfleet2/internal/contract"
)

// progressThrottle 节点侧回调节流间隔。
const progressThrottle = 500 * time.Millisecond

// emitInterval 聚合快照的渲染节流间隔。
const emitInterval = 200 * time.Millisecond

// nodeProgress 单节点进度快照。
type nodeProgress struct {
	Seq          int
	IP           string
	Bytes        int64 // 已传输字节（只增不减，取节点自报的最大值）
	TotalBytes   int64
	TotalFiles   int
	SuccessFiles int
	FailedFiles  int
	Done         bool
}

// snapshot 聚合快照。
type snapshot struct {
	Completed  int
	Succeeded  int
	Failed     int
	BytesDone  int64
	BytesTotal int64
	Nodes      []nodeProgress
}

// aggregator 进度聚合器：跨调用保存状态，与执行同生命周期。
type aggregator struct {
	mu         sync.Mutex
	completed  int
	succeeded  int
	failed     int
	bytesDone  int64
	bytesTotal int64
	nodes      map[int]*nodeProgress
	lastEmit   time.Time
	render     func(snapshot)
}

func newAggregator(render func(snapshot)) *aggregator {
	return &aggregator{nodes: make(map[int]*nodeProgress), render: render}
}

// onProgress 接收单节点进度。
func (ag *aggregator) onProgress(n nodeProgress) {
	ag.mu.Lock()
	node := ag.node(n.Seq)
	node.IP = n.IP
	if n.TotalBytes > 0 && n.TotalBytes != node.TotalBytes {
		ag.bytesTotal += n.TotalBytes - node.TotalBytes
		node.TotalBytes = n.TotalBytes
	}
	if n.Bytes > node.Bytes {
		ag.bytesDone += n.Bytes - node.Bytes
		node.Bytes = n.Bytes
	}
	if n.TotalFiles > 0 {
		node.TotalFiles = n.TotalFiles
	}
	node.SuccessFiles = n.SuccessFiles
	node.FailedFiles = n.FailedFiles
	snap := ag.snapshotLocked()
	ag.mu.Unlock()
	ag.emit(snap, false)
}

// onResult 接收节点完成结果（成败由调用方判好传进来；聚合器不做第二套判法）。
func (ag *aggregator) onResult(seq int, ip string, success bool, t contract.TransferStat) {
	ag.mu.Lock()
	node := ag.node(seq)
	node.IP = ip
	node.Done = true
	if t.TotalBytes > node.Bytes {
		ag.bytesDone += t.TotalBytes - node.Bytes
		node.Bytes = t.TotalBytes
	}
	if t.TotalBytes > node.TotalBytes {
		ag.bytesTotal += t.TotalBytes - node.TotalBytes
		node.TotalBytes = t.TotalBytes
	}
	node.TotalFiles = t.TotalFiles
	node.SuccessFiles = t.SuccessFiles
	node.FailedFiles = t.FailFiles

	ag.completed++
	if success {
		ag.succeeded++
	} else {
		ag.failed++
	}
	snap := ag.snapshotLocked()
	ag.mu.Unlock()
	ag.emit(snap, true)
}

func (ag *aggregator) node(seq int) *nodeProgress {
	n, ok := ag.nodes[seq]
	if !ok {
		n = &nodeProgress{Seq: seq}
		ag.nodes[seq] = n
	}
	return n
}

func (ag *aggregator) snapshotLocked() snapshot {
	snap := snapshot{
		Completed:  ag.completed,
		Succeeded:  ag.succeeded,
		Failed:     ag.failed,
		BytesDone:  ag.bytesDone,
		BytesTotal: ag.bytesTotal,
		Nodes:      make([]nodeProgress, 0, len(ag.nodes)),
	}
	for _, n := range ag.nodes {
		snap.Nodes = append(snap.Nodes, *n)
	}
	sort.Slice(snap.Nodes, func(i, j int) bool { return snap.Nodes[i].Seq < snap.Nodes[j].Seq })
	return snap
}

// emit 按节流间隔调用渲染函数；force=true（节点完成）时立即渲染。
func (ag *aggregator) emit(snap snapshot, force bool) {
	if ag.render == nil {
		return
	}
	ag.mu.Lock()
	if !force && time.Since(ag.lastEmit) < emitInterval {
		ag.mu.Unlock()
		return
	}
	ag.lastEmit = time.Now()
	ag.mu.Unlock()
	ag.render(snap)
}

// —— 节点侧 ——

// progressPacer 循环级逐文件进度节流器（零值可用）。
type progressPacer struct{ last time.Time }

func (p *progressPacer) allow() bool {
	now := time.Now()
	if now.Sub(p.last) >= progressThrottle {
		p.last = now
		return true
	}
	return false
}

// progressWriter 带进度回调的写入器（上传路径）。
type progressWriter struct {
	dst          io.Writer
	uploaded     int64
	lastCallback time.Time
	seq          int
	ip           string
	totalBytes   int64
	totalFiles   int
	callback     func(nodeProgress)
	mu           sync.Mutex
}

func (pw *progressWriter) Write(p []byte) (int, error) {
	n, err := pw.dst.Write(p)
	pw.mu.Lock()
	pw.uploaded += int64(n)
	if pw.callback != nil && time.Since(pw.lastCallback) >= progressThrottle {
		pw.lastCallback = time.Now()
		pw.callback(nodeProgress{Seq: pw.seq, IP: pw.ip, Bytes: pw.uploaded, TotalBytes: pw.totalBytes, TotalFiles: pw.totalFiles})
	}
	pw.mu.Unlock()
	return n, err
}

// progressReader 带进度回调的读取器（下载路径）。
type progressReader struct {
	src          io.Reader
	downloaded   int64
	lastCallback time.Time
	seq          int
	ip           string
	totalBytes   int64
	totalFiles   int
	callback     func(nodeProgress)
	mu           sync.Mutex
}

func (pr *progressReader) Read(p []byte) (int, error) {
	n, err := pr.src.Read(p)
	pr.mu.Lock()
	pr.downloaded += int64(n)
	should := time.Since(pr.lastCallback) >= progressThrottle
	if err == io.EOF {
		should = true
	}
	if should && pr.callback != nil {
		pr.lastCallback = time.Now()
		pr.callback(nodeProgress{Seq: pr.seq, IP: pr.ip, Bytes: pr.downloaded, TotalBytes: pr.totalBytes, TotalFiles: pr.totalFiles})
	}
	pr.mu.Unlock()
	return n, err
}

// copyUpload 1MB 缓冲流式上传；无回调时直通（不走进度包裹）。
func copyUpload(dst io.Writer, src io.Reader, cb onProgressN, seq int, ip string, totalBytes int64, totalFiles int) (int64, error) {
	buf := make([]byte, 1024*1024)
	if cb == nil {
		return io.CopyBuffer(dst, src, buf)
	}
	return io.CopyBuffer(&progressWriter{dst: dst, seq: seq, ip: ip, totalBytes: totalBytes, totalFiles: totalFiles, callback: cb}, src, buf)
}

// copyDownload 1MB 缓冲流式下载；无回调时直通。
func copyDownload(dst io.Writer, src io.Reader, cb onProgressN, seq int, ip string, totalBytes int64, totalFiles int) (int64, error) {
	buf := make([]byte, 1024*1024)
	if cb == nil {
		return io.CopyBuffer(dst, src, buf)
	}
	return io.CopyBuffer(dst, &progressReader{src: src, seq: seq, ip: ip, totalBytes: totalBytes, totalFiles: totalFiles, callback: cb}, buf)
}

// onProgressN 给上传／下载用的便捷回调：把逐文件的四个数与字节一起报上去。
type onProgressN func(n nodeProgress)

// reportTransfer 把一次文件级进展报上去（pacer 节流由调用方掌握）。
func reportTransfer(cb onProgressN, seq int, ip string, uploaded, totalBytes int64, totalFiles, success, failed int) {
	if cb == nil {
		return
	}
	cb(nodeProgress{
		Seq: seq, IP: ip, Bytes: uploaded, TotalBytes: totalBytes,
		TotalFiles: totalFiles, SuccessFiles: success, FailedFiles: failed,
	})
}
