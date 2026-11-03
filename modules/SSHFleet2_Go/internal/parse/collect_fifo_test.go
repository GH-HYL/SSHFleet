//go:build unix

package parse

// FIFO 那一类条目（建站 05 的 06 号单）：它们不是「链接」，过滤掉会静默改变语义，
// 所以按错报。造一个 FIFO 要系统调用，这一段只在 unix 上跑。

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestCollectRejectsFIFO(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "dist")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatalf("造目录失败：%v", err)
	}
	fifo := filepath.Join(root, "pipe")
	if err := syscall.Mkfifo(fifo, 0o644); err != nil {
		t.Skipf("这一台造不出 FIFO：%v", err)
	}
	if _, fail := collectPlan(t, root); fail == nil {
		t.Fatal("FIFO 应当按错报，不能静默过滤")
	}
}
