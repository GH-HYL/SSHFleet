package execute

// 传输类的进度：节点侧的回调节流、跨节点的聚合，以及那块逐节点的面板。
//
// 三层各管一件事（与老版同一套分工）：
//   - 字节／文件级回调（progressCounter／pacer）只管「把数报上去」，500ms 节流；
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

// progressCounter 逐字节的回调计数：包在「读出来」或「写进去」的那一头上，按节流间隔报一次。
//
// 上传数进去的、下载数出来的，两边只差数的是哪一头，所以判断与计数只有这一处。读侧还要在
// 流到了头时补一帧：文件小、字节级回调太稀时，面板靠这一帧落到终值；写侧看不见「到头」，
// 那一帧只属于读侧。
type progressCounter struct {
	done       int64
	last       time.Time
	seq        int
	ip         string
	totalBytes int64
	totalFiles int
	callback   func(nodeProgress)
	mu         sync.Mutex
}

func newProgressCounter(cb func(nodeProgress), seq int, ip string, totalBytes int64, totalFiles int) *progressCounter {
	return &progressCounter{seq: seq, ip: ip, totalBytes: totalBytes, totalFiles: totalFiles, callback: cb}
}

// add 记下这次读／写的字节数；到点就报一帧。
func (c *progressCounter) add(n int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.done += int64(n)
	if time.Since(c.last) < progressThrottle {
		return
	}
	c.last = time.Now()
	c.frame()
}

// flush 立刻报一帧：读侧读到流末尾时用，不看节流。
func (c *progressCounter) flush() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.last = time.Now()
	c.frame()
}

func (c *progressCounter) frame() {
	if c.callback == nil {
		return
	}
	c.callback(nodeProgress{
		Seq: c.seq, IP: c.ip, Bytes: c.done, TotalBytes: c.totalBytes, TotalFiles: c.totalFiles,
	})
}

// countingWriter 把写进 dst 的字节数报上去（上传那一头）。
type countingWriter struct {
	io.Writer
	c *progressCounter
}

func (w countingWriter) Write(p []byte) (int, error) {
	n, err := w.Writer.Write(p)
	w.c.add(n)
	return n, err
}

// countingReader 把从 src 读到的字节数报上去，读到末尾补一帧（下载那一头）。
type countingReader struct {
	io.Reader
	c *progressCounter
}

func (r countingReader) Read(p []byte) (int, error) {
	n, err := r.Reader.Read(p)
	r.c.add(n)
	if err == io.EOF {
		r.c.flush()
	}
	return n, err
}

// transDirection 这一趟的字节往哪边走：客户端写给目标机，还是目标机给到本机。
type transDirection int

const (
	sending   transDirection = iota // 往目标机送（上传）
	receiving                       // 从目标机收（下载）
)

// copyStream 1MB 缓冲流式拷贝；无回调时直通（不走进度包裹）。
func copyStream(dir transDirection, dst io.Writer, src io.Reader, cb onProgressN, seq int, ip string, totalBytes int64, totalFiles int) (int64, error) {
	buf := make([]byte, 1024*1024)
	if cb == nil {
		return io.CopyBuffer(dst, src, buf)
	}
	c := newProgressCounter(cb, seq, ip, totalBytes, totalFiles)
	if dir == sending {
		return io.CopyBuffer(countingWriter{Writer: dst, c: c}, src, buf)
	}
	return io.CopyBuffer(dst, countingReader{Reader: src, c: c}, buf)
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
