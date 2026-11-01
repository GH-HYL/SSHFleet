package parse

import "testing"

// 认「要不要看帮助」：-h / --help / 空参三种触发；带值选项吃掉下一个 token；
// 不认识的选项当场停；-- 之后不算；位置参数不拦。

func TestHelpRequested(t *testing.T) {
	cases := []struct {
		name string
		argv []string
		want bool
	}{
		{"空参数", []string{"sshfleet"}, true},
		{"短帮助", []string{"sshfleet", "-h"}, true},
		{"长帮助", []string{"sshfleet", "--help"}, true},
		{"帮助在值后面", []string{"sshfleet", "-f", "a.csv", "-h"}, true},
		{"值恰好是 -h", []string{"sshfleet", "-c", "-h"}, false},
		{"带等号的取值里带 -h", []string{"sshfleet", "-c=-h"}, false},
		{"位置参数后面的帮助", []string{"sshfleet", "misc", "-h"}, true},
		{"双横线之后不算", []string{"sshfleet", "--", "-h"}, false},
		{"不认识的选项当场停", []string{"sshfleet", "--bogus", "-h"}, false},
		{"只有别的选项", []string{"sshfleet", "--yes"}, false},
		{"布尔开关后面的帮助", []string{"sshfleet", "--yes", "-h"}, true},
		{"值本身长得像选项但认得出", []string{"sshfleet", "--command", "-h"}, false},
	}
	for _, c := range cases {
		if got := helpRequested(c.argv); got != c.want {
			t.Errorf("%s：want %v，got %v（argv %v）", c.name, c.want, got, c.argv)
		}
	}
}
