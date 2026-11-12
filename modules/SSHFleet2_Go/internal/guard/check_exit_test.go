package guard

import (
	"strings"
	"testing"
)

// 类三：塔尖之外取用 os.Exit 要报；「打日志再退」那种收口也要报；塔尖里用不报。
//
// 判的是取用不是调用（ADR-0048）：样本里 gate 把 os.Exit 存进一个包级变量、一次都没调，
// 照样报。
func TestCheckExit(t *testing.T) {
	vs := inClass(CheckExit(loadFixture(t, "violations")), ClassExit)
	if len(vs) != 3 {
		t.Fatalf("want 3 条（当场调用、存成函数值、log.Fatal），got %d：\n%s", len(vs), Report(vs))
	}
	if !anyContains(vs, "os.Exit") {
		t.Errorf("os.Exit 没报出来：\n%s", Report(vs))
	}
	if !anyContains(vs, "log.Fatal") {
		t.Errorf("「打日志再退」那种收口没报出来：\n%s", Report(vs))
	}
	if !anyContains(vs, "internal/gate/exit.go") {
		t.Errorf("存成函数值那一处没报出来：\n%s", Report(vs))
	}
	for _, v := range vs {
		if !strings.Contains(v.Fix, "塔尖") {
			t.Errorf("话术要说清退出只归塔尖：%s", v)
		}
	}

	if got := CheckExit(loadFixture(t, "clean")); len(got) != 0 {
		t.Errorf("clean 样本里 os.Exit 只在塔尖，应当零违规，got：%v", got)
	}
}
