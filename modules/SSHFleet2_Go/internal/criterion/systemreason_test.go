package criterion

import (
	"errors"
	"io/fs"
	"os"
	"testing"
)

// 剥系统前缀：只剥掉「哪一段是路径」，剩下的原话一字不动。
//
// 三支都要过——凭据包那份曾经只剥 `*fs.PathError`，与老版 `common.SystemReason` 漂开，
// 而没有任何测试会红（ADR-0051）。

func TestSystemReason(t *testing.T) {
	pathErr := &fs.PathError{Op: "open", Path: `/tmp/nodes.csv`, Err: fs.ErrNotExist}
	if got := SystemReason(pathErr); got != fs.ErrNotExist.Error() {
		t.Errorf("路径前缀没剥掉：got %q，want %q", got, fs.ErrNotExist.Error())
	}

	linkErr := &os.LinkError{Op: "rename", Old: "a", New: "b", Err: fs.ErrPermission}
	if got := SystemReason(linkErr); got != fs.ErrPermission.Error() {
		t.Errorf("链接类前缀没剥掉：got %q，want %q", got, fs.ErrPermission.Error())
	}

	plain := errors.New("连接被拒绝")
	if got := SystemReason(plain); got != "连接被拒绝" {
		t.Errorf("别的错误应当原样交出：got %q", got)
	}
}
