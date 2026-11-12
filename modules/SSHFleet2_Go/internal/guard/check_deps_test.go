package guard

import (
	"strings"
	"testing"
)

// 类二：生产文件按规则表判；包清单多一个、少一个也归这一类。
//
// 空白 import 与点 import 也算边（ADR-0048）：样本里那两条写成 `_` 与 `.`，
// 依赖方向一样成立，判据不认写法。
func TestCheckDeps(t *testing.T) {
	vs := CheckDeps(loadFixture(t, "violations"))
	if len(vs) != 9 {
		t.Fatalf("want 9 条，got %d：\n%s", len(vs), Report(vs))
	}
	for _, want := range []string{
		"parse 依赖了 gate",         // 站依赖站
		"message 依赖了 parse",      // 横切反向依赖站
		"example.com/x",          // 第三方依赖
		"fixture/internal/extra", // 多一个包
		"log 在工程里不存在",            // 少一个包
		"os.Getenv",              // 塔尖用了没点名的符号
		"gate 依赖了 ask",          // 点 import：名字没写，边还在
		"parse 依赖了 ask",         // 空白 import：同上
	} {
		if !anyContains(vs, want) {
			t.Errorf("少了这一条：%q，实得：\n%s", want, Report(vs))
		}
	}
	for _, v := range vs {
		if strings.HasSuffix(v.File, "side.go") && !strings.Contains(v.What, "依赖了") {
			t.Errorf("绕法样本应当报成依赖边：%s", v)
		}
	}
}

// _test.go 豁免依赖方向，塔尖 import 五站也合法。
func TestCheckDepsClean(t *testing.T) {
	if vs := CheckDeps(loadFixture(t, "clean")); len(vs) != 0 {
		t.Fatalf("clean 样本应当零违规，got：\n%s", Report(vs))
	}
}

// 类二第三条：读当前时刻只许塔尖、执行站与日志包；解析站读一次就报。
//
// 同一个样本证明两半：那三个家各写一次 time.Now 不红，第四个包写一次就红。
func TestCheckDepsClock(t *testing.T) {
	wantOne(t, CheckDeps(loadFixture(t, "clock")), ClassDeps, "internal/parse/run.go", "time.Now")
}

func anyContains(vs []Violation, sub string) bool {
	for _, v := range vs {
		if strings.Contains(v.String(), sub) {
			return true
		}
	}
	return false
}
