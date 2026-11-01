package guard

import "testing"

// 类六之一：红线测试钉两个包要报；站导出第二个类型也要报。
func TestCheckTestSite(t *testing.T) {
	vs := CheckTestSite(loadFixture(t, "testsite"))
	if len(vs) != 2 {
		t.Fatalf("want 2 条（钉两个包、站导出两个类型），got %d：\n%s", len(vs), Report(vs))
	}
	if !anyContains(vs, "钉了 2 个包") {
		t.Errorf("红线测试钉两个包没报出来：\n%s", Report(vs))
	}
	if !anyContains(vs, "导出了 2 个类型") {
		t.Errorf("站导出第二个类型没报出来：\n%s", Report(vs))
	}
}

// 骨架阶段 0 个导出也算过；一条测试钉一个包也合法。
func TestCheckTestSiteClean(t *testing.T) {
	if vs := CheckTestSite(loadFixture(t, "clean")); len(vs) != 0 {
		t.Fatalf("clean 样本应当零违规，got：\n%s", Report(vs))
	}
}
