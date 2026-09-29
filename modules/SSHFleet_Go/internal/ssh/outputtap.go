package ssh

import (
	"strings"
	"sync"
)

// outputTap 采集侧的共同前半段（代填与改密共用）。
//
// session.Stdout 与 session.Stderr 各由一个复制协程推来，所以写入要带锁；结尾的 `\r`
// 先留住——它后面可能跟一个 `\n`，而两者会被切在相邻两块里。
// 整备好的**同一份**字节既落缓冲、又交给 onChunk：缓冲与匹配器必须看到同一份字节，
// 游标才有意义。分叉的只是 onChunk 里的匹配语义（代填一次性消费，改密可重复）。
type outputTap struct {
	mu      sync.Mutex
	out     *lockedBuffer
	held    string
	onChunk func(string)
}

func newOutputTap(out *lockedBuffer, onChunk func(string)) *outputTap {
	return &outputTap{out: out, onChunk: onChunk}
}

// Write 采集侧整备：去行尾 `\r` → 写进缓冲 → 喂匹配。
func (t *outputTap) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.appendLocked(string(p))
	return len(p), nil
}

// Flush 把待定的 `\r` 交出去（会话结束后调用一次）。
func (t *outputTap) Flush() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.held == "" {
		return
	}
	chunk := t.held
	t.held = ""
	t.emitLocked(chunk)
}

// locked 在采集侧那把锁里做一件事（读匹配器状态这类要与写入串行的操作）。
func (t *outputTap) locked(fn func()) {
	t.mu.Lock()
	defer t.mu.Unlock()
	fn()
}

func (t *outputTap) appendLocked(s string) {
	chunk := t.held + s
	t.held = ""
	if strings.HasSuffix(chunk, "\r") {
		chunk, t.held = chunk[:len(chunk)-1], "\r"
	}
	t.emitLocked(strings.ReplaceAll(chunk, "\r\n", "\n"))
}

func (t *outputTap) emitLocked(chunk string) {
	if chunk == "" {
		return
	}
	_, _ = t.out.Write([]byte(chunk))
	t.onChunk(chunk)
}
