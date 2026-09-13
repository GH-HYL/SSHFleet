package batch

import (
	"sort"
	"sync"
	"time"

	"sshfleet/internal/ssh"
)

// emitInterval 聚合快照的渲染节流间隔（各节点自身进度已按 500ms 节流，
// 这里再兜一层，避免多节点同时上报时刷屏）。
const emitInterval = 200 * time.Millisecond

// NodeSnapshot 单节点进度快照。
type NodeSnapshot struct {
	Seq            int
	IP             string
	Bytes          int64 // 已传输字节（只增不减，取节点自报的最大值）
	TotalBytes     int64
	TotalFiles     int
	SuccessFiles   int
	FailedFiles    int
	Done           bool
	ConnectSuccess bool
	ExitCode       *int
}

// Snapshot 聚合快照（渲染函数的输入；M5 据它画总进度与各节点进度）。
type Snapshot struct {
	Total      int
	Completed  int
	Succeeded  int
	Failed     int
	BytesDone  int64
	BytesTotal int64
	Nodes      []NodeSnapshot
}

// Aggregator 进度聚合器：汇总各节点的字节 / 文件数 / 完成数，
// 跨调用保存状态、与 worker pool 同一生命周期（spec D2、D32 条件 1）。
type Aggregator struct {
	mu         sync.Mutex
	total      int
	completed  int
	succeeded  int
	failed     int
	bytesDone  int64
	bytesTotal int64
	nodes      map[int]*NodeSnapshot
	lastEmit   time.Time
	render     RenderFunc
}

// NewAggregator 创建聚合器；render 可为 nil（无渲染需求，如测试）。
func NewAggregator(total int, render RenderFunc) *Aggregator {
	return &Aggregator{total: total, nodes: make(map[int]*NodeSnapshot), render: render}
}

// OnProgress 接收单节点进度（ssh 的字节级 / 循环级回调）。
func (ag *Aggregator) OnProgress(p ssh.Progress) {
	ag.mu.Lock()
	node := ag.node(p.Seq)
	node.IP = p.IP
	if p.TotalBytes > 0 {
		// 节点自报总量只增不减，聚合侧同步增量
		if p.TotalBytes != node.TotalBytes {
			ag.bytesTotal += p.TotalBytes - node.TotalBytes
			node.TotalBytes = p.TotalBytes
		}
	}
	bytes := p.UploadedBytes
	if p.DownloadedBytes > bytes {
		bytes = p.DownloadedBytes
	}
	if bytes > node.Bytes {
		ag.bytesDone += bytes - node.Bytes
		node.Bytes = bytes
	}
	if p.TotalFiles > 0 {
		node.TotalFiles = p.TotalFiles
	}
	node.SuccessFiles = p.SuccessFiles
	node.FailedFiles = p.FailedFiles
	snap := ag.snapshotLocked()
	ag.mu.Unlock()
	ag.emit(snap, false)
}

// OnResult 接收节点完成结果（更新完成数、成功 / 失败、最终字节）。
func (ag *Aggregator) OnResult(r ssh.Result) {
	ag.mu.Lock()
	node := ag.node(r.Seq)
	node.IP = r.IP
	node.Done = true
	node.ConnectSuccess = r.ConnectSuccess
	node.ExitCode = r.ExitCode
	if r.TotalBytes > node.Bytes {
		ag.bytesDone += r.TotalBytes - node.Bytes
		node.Bytes = r.TotalBytes
	}
	if r.TotalBytes > node.TotalBytes {
		ag.bytesTotal += r.TotalBytes - node.TotalBytes
		node.TotalBytes = r.TotalBytes
	}
	node.TotalFiles = r.TotalFiles
	node.SuccessFiles = r.SuccessFiles
	node.FailedFiles = r.FailedFiles

	ag.completed++
	if r.ExitCode != nil && *r.ExitCode == 0 {
		ag.succeeded++
	} else {
		ag.failed++
	}
	snap := ag.snapshotLocked()
	ag.mu.Unlock()
	ag.emit(snap, true)
}

// Snapshot 当前聚合快照（含所有节点）。
func (ag *Aggregator) Snapshot() Snapshot {
	ag.mu.Lock()
	defer ag.mu.Unlock()
	return ag.snapshotLocked()
}

func (ag *Aggregator) node(seq int) *NodeSnapshot {
	n, ok := ag.nodes[seq]
	if !ok {
		n = &NodeSnapshot{Seq: seq}
		ag.nodes[seq] = n
	}
	return n
}

func (ag *Aggregator) snapshotLocked() Snapshot {
	snap := Snapshot{
		Total:      ag.total,
		Completed:  ag.completed,
		Succeeded:  ag.succeeded,
		Failed:     ag.failed,
		BytesDone:  ag.bytesDone,
		BytesTotal: ag.bytesTotal,
		Nodes:      make([]NodeSnapshot, 0, len(ag.nodes)),
	}
	for _, n := range ag.nodes {
		snap.Nodes = append(snap.Nodes, *n)
	}
	sort.Slice(snap.Nodes, func(i, j int) bool { return snap.Nodes[i].Seq < snap.Nodes[j].Seq })
	return snap
}

// emit 按节流间隔调用渲染函数；force=true（节点完成）时立即渲染。
func (ag *Aggregator) emit(snap Snapshot, force bool) {
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
