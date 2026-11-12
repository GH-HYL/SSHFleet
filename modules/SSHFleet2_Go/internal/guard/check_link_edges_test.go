package guard

import (
	"strings"
	"testing"
)

// 类二第四条：不经 import 的边也报——`//go:linkname` 一处、汇编文件一份。
//
// 这两样是唯一能「完全不经 import 绑别人家符号」的机制：不禁掉它们，import 表只等于
// 「愿意登记的边」（ADR-0049）。
func TestCheckLinkEdges(t *testing.T) {
	vs := CheckDeps(loadFixture(t, "linkedge"))
	if len(vs) != 2 {
		t.Fatalf("want 2 条（linkname、汇编），got %d：\n%s", len(vs), Report(vs))
	}
	if !anyContains(vs, "go:linkname") {
		t.Errorf("linkname 那一条没报出来：\n%s", Report(vs))
	}
	if !anyContains(vs, "hop.s") {
		t.Errorf("汇编文件没报出来：\n%s", Report(vs))
	}
	for _, v := range vs {
		if v.Class != ClassDeps {
			t.Errorf("该归类二，实际 %s：%s", v.Class, v)
		}
		if !strings.Contains(v.Fix, "ADR-0032") {
			t.Errorf("话术要说清放开它得改红线：%s", v)
		}
	}
}
