package guard

import (
	"testing"

	"sshfleet2/internal/exitcode"
)

// 红线：exitcode 的那一件事（种类换码）。
//
// 判关系不判数值（ADR-0056）：零／非零是使用者感觉得到的——`if sshfleet; then` 判的就是
// 这一位；具体哪个数归本包的自定义区（`internal/exitcode/exitcode_test.go` 一张表钉四个数），
// 改码只红那一条，不红塔尖。
//
// 另留一条老版既有行为：两种失败同码——它不是「哪个数」，是「这两件事在退出码这一面不区分」，
// 改动它等于改使用者看得见的收场分类。

func TestRedlineExitCodeTable(t *testing.T) {
	if got := exitcode.Code(exitcode.Success); got != 0 {
		t.Errorf("成功收场应以 0 退出，got %d", got)
	}
	for _, k := range []exitcode.Kind{exitcode.CannotStart, exitcode.Refused, exitcode.Interrupted} {
		if exitcode.Code(k) == 0 {
			t.Errorf("种类 %v 不能以 0 退出：0 只留给成功", k)
		}
	}
	if a, b := exitcode.Code(exitcode.CannotStart), exitcode.Code(exitcode.Refused); a != b {
		t.Errorf("「起不来」与「拒绝」应同码（老版既有行为）：%d 与 %d", a, b)
	}
}
