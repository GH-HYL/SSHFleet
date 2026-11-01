package parse

import "testing"

// 版本串四档退化 + dirty。期望值都是写死的字面量——号、提交号、时间的拼法
// 逐字照老工程 main.go 的 versionWithBuildID。

func TestVersionString(t *testing.T) {
	cases := []struct {
		name         string
		num, rev, tm string
		dirty        bool
		want         string
	}{
		{"有提交号与时间", "1.0.0", "0123456789abcdef", "2026-11-01T13:00:00Z", false, "v1.0.0+0123456 (2026-11-01 21:00:00)"},
		{"只有提交号", "1.0.0", "0123456789abcdef", "", false, "v1.0.0+0123456"},
		{"只有时间", "1.0.0", "", "2026-11-01T13:00:00Z", false, "v1.0.0 (2026-11-01 21:00:00)"},
		{"都没有", "1.0.0", "", "", false, "v1.0.0"},
		{"有未提交改动", "1.0.0", "0123456789abcdef", "2026-11-01T13:00:00Z", true, "v1.0.0+0123456-dirty (2026-11-01 21:00:00)"},
		{"时间解析不出来按没有算", "1.0.0", "0123456789abcdef", "不是时间", false, "v1.0.0+0123456"},
		{"短提交号取前 7 位", "2.3.4", "abcdef1234567890", "", false, "v2.3.4+abcdef1"},
	}
	for _, c := range cases {
		if got := versionString(c.num, c.rev, c.tm, c.dirty); got != c.want {
			t.Errorf("%s：want %q，got %q", c.name, c.want, got)
		}
	}
}
