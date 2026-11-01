package guard

import (
	"testing"

	"sshfleet2/internal/exitcode"
)

// 红线：exitcode 的那一件事（种类换码）。期望值是写死的四个数——与老版逐项一致。

func TestRedlineExitCodeTable(t *testing.T) {
	cases := []struct {
		kind exitcode.Kind
		want int
	}{
		{exitcode.Success, 0},
		{exitcode.CannotStart, 1},
		{exitcode.Refused, 1},
		{exitcode.Interrupted, 130},
	}
	for _, c := range cases {
		if got := exitcode.Code(c.kind); got != c.want {
			t.Errorf("种类 %v 应换出 %d，got %d", c.kind, c.want, got)
		}
	}
}
