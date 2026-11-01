package guard

import (
	"strings"
	"testing"
)

// 类三：塔尖之外调 os.Exit 要报；「打日志再退」那种收口也要报；塔尖里调不报。
func TestCheckExit(t *testing.T) {
	vs := inClass(CheckExit(loadFixture(t, "violations")), ClassExit)
	if len(vs) != 2 {
		t.Fatalf("want 2 条（os.Exit、log.Fatal），got %d：\n%s", len(vs), Report(vs))
	}
	if !anyContains(vs, "os.Exit") {
		t.Errorf("os.Exit 没报出来：\n%s", Report(vs))
	}
	if !anyContains(vs, "log.Fatal") {
		t.Errorf("「打日志再退」那种收口没报出来：\n%s", Report(vs))
	}
	for _, v := range vs {
		if v.File != "internal/parse/run.go" || !strings.Contains(v.Fix, "塔尖") {
			t.Errorf("位置或话术不对：%s", v)
		}
	}

	if got := CheckExit(loadFixture(t, "clean")); len(got) != 0 {
		t.Errorf("clean 样本里 os.Exit 只在塔尖，应当零违规，got：%v", got)
	}
}
