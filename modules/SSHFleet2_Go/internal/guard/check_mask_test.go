package guard

import (
	"strings"
	"testing"
)

// 类七：遮的构造点只有一个。
//
// 正样本是本包自己写星号（`mask` 包），违规样本是解析站自己替换明文，`_test.go` 不扫。
func TestCheckMask(t *testing.T) {
	vs := inClass(CheckMask(loadFixture(t, "mask")), ClassMask)
	if len(vs) != 1 {
		t.Fatalf("want 1 条（parse 自己拼星号），got %d：\n%s", len(vs), Report(vs))
	}
	v := vs[0]
	if v.File != "internal/parse/run.go" {
		t.Errorf("位置不对：%s", v)
	}
	if !strings.Contains(v.Fix, "mask") {
		t.Errorf("该走的话术没指到 mask：%s", v)
	}

	if got := inClass(CheckMask(loadFixture(t, "clean")), ClassMask); len(got) != 0 {
		t.Errorf("clean 样本里没人自己遮，应当零违规：%v", got)
	}
}
