package guard

import (
	"strings"
	"testing"
)

// 类二：生产文件按规则表判；包清单多一个、少一个也归这一类。
func TestCheckDeps(t *testing.T) {
	vs := CheckDeps(loadFixture(t, "violations"))
	if len(vs) != 6 {
		t.Fatalf("want 6 条，got %d：\n%s", len(vs), Report(vs))
	}
	for _, want := range []string{
		"parse 依赖了 gate",         // 站依赖站
		"message 依赖了 parse",      // 横切反向依赖站
		"example.com/x",          // 第三方依赖
		"fixture/internal/extra", // 多一个包
		"log 在工程里不存在",            // 少一个包
		"os.Getenv",              // 塔尖用了没点名的符号
	} {
		if !anyContains(vs, want) {
			t.Errorf("少了这一条：%q，实得：\n%s", want, Report(vs))
		}
	}
}

// _test.go 豁免依赖方向，塔尖 import 五站也合法。
func TestCheckDepsClean(t *testing.T) {
	if vs := CheckDeps(loadFixture(t, "clean")); len(vs) != 0 {
		t.Fatalf("clean 样本应当零违规，got：\n%s", Report(vs))
	}
}

func anyContains(vs []Violation, sub string) bool {
	for _, v := range vs {
		if strings.Contains(v.String(), sub) {
			return true
		}
	}
	return false
}
