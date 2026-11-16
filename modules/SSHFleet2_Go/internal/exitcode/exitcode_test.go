package exitcode

import "testing"

// 这一条判「哪种收场换哪个数」——是值，住被测包的自定义区（ADR-0056）。
//
// 期望值是写死的四个数，与老版逐项一致：老版只有 0／1／130 三个值，「拒绝」与「起不来」
// 同码是既有行为，不是笔误。这几个数原来只挂在塔尖的红线上——改一次码就红一次塔尖，
// 红的原因还写着「结构被动了」。搬到这里之后，改码只红这一条。
//
// 新增一种收场时这张表要跟着加一行；漏了不会有人报（红线只判零／非零），所以加 `Kind`
// 的时候顺手看一眼这里。
func TestCodeTable(t *testing.T) {
	cases := []struct {
		kind Kind
		want int
	}{
		{Success, 0},
		{CannotStart, 1},
		{Refused, 1},
		{Interrupted, 130},
	}
	for _, c := range cases {
		if got := Code(c.kind); got != c.want {
			t.Errorf("种类 %v 应换出 %d，got %d", c.kind, c.want, got)
		}
	}
}
