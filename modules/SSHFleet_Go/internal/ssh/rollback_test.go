package ssh

// 回滚的删除动作本身：只删点名的路径、删不掉要报出来、绝不扩大范围。
//
// 回滚是删文件的动作——删错比不删更糟，所以这里不看「返回了什么」，看「删了哪些、
// 漏了哪些」。删除函数由外部注入，专测「删不掉」这条真实环境里难造的分支。

import (
	"errors"
	"strings"
	"testing"
)

func TestRollbackWithDeletesExactlyTheNamedPaths(t *testing.T) {
	var called []string
	msg := rollbackWith([]string{"/data/a.bin", "/data/b.bin"}, func(p string) error {
		called = append(called, p)
		return nil
	})

	if len(called) != 2 || called[0] != "/data/a.bin" || called[1] != "/data/b.bin" {
		t.Fatalf("只该删点名的这两条，实际 %v", called)
	}
	if want := "sshfleet.transfer.rolled_back: 2 of 2 written files removed"; msg != want {
		t.Fatalf("报文 = %q，期望 %q", msg, want)
	}
}

// 清单为空时一条都不删，也不说话——目标机本来就没留下东西。
func TestRollbackWithEmptyListDoesNothing(t *testing.T) {
	called := 0
	msg := rollbackWith(nil, func(string) error {
		called++
		return nil
	})
	if called != 0 {
		t.Fatalf("没有要删的路径时不该调用删除，实际调用 %d 次", called)
	}
	if msg != "" {
		t.Fatalf("没有要删的路径时不该发报文，实际 %q", msg)
	}
}

// 删不掉的路径必须留在报文里，别假装清干净了。
func TestRollbackWithReportsLeftovers(t *testing.T) {
	msg := rollbackWith([]string{"/data/a.bin", "/data/locked.bin"}, func(p string) error {
		if p == "/data/locked.bin" {
			return errors.New("permission denied")
		}
		return nil
	})

	if !strings.HasPrefix(msg, "sshfleet.transfer.rollback_partial: ") {
		t.Fatalf("删不掉时该发 rollback_partial，实际 %q", msg)
	}
	if !strings.Contains(msg, "1 of 2 written files removed") {
		t.Fatalf("应如实说清删掉几个，实际 %q", msg)
	}
	if !strings.Contains(msg, "/data/locked.bin") {
		t.Fatalf("应把没删掉的路径列出来，实际 %q", msg)
	}
	if strings.Contains(msg, "/data/a.bin") {
		t.Fatalf("删掉的路径不该混进「剩下哪些」里，实际 %q", msg)
	}
}

// 一个都删不掉时同样要说话，不能返回空串被当成「无事可陈」。
func TestRollbackWithAllFailuresStillSpeaks(t *testing.T) {
	msg := rollbackWith([]string{"/data/a.bin"}, func(string) error {
		return errors.New("connection lost")
	})
	if !strings.Contains(msg, "0 of 1 written files removed") {
		t.Fatalf("全删不掉也要说清，实际 %q", msg)
	}
}

// 半成品没删掉时的那句话：空清单不产出，有路径就列出来（工单 09）。
func TestPartialLeftMessage(t *testing.T) {
	if got := PartialLeftMessage(nil); got != "" {
		t.Fatalf("没有半成品时不该产出报文，得到 %q", got)
	}
	got := PartialLeftMessage([]string{"/data/app/x.bin"})
	want := "sshfleet.transfer.partial_left: half-written file left: /data/app/x.bin"
	if got != want {
		t.Fatalf("报文 = %q，期望 %q", got, want)
	}

	if got := LocalPartialLeftMessage(nil); got != "" {
		t.Fatalf("没有半成品时不该产出报文，得到 %q", got)
	}
	gotLocal := LocalPartialLeftMessage([]string{"D:/logs/x.bin"})
	wantLocal := "sshfleet.transfer.local_partial_left: half-written file left: D:/logs/x.bin"
	if gotLocal != wantLocal {
		t.Fatalf("报文 = %q，期望 %q", gotLocal, wantLocal)
	}
}
