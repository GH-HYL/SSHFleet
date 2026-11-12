package guard

import (
	"strings"
	"testing"
)

// 类五：闸门里造「起不来」要报；闸门外造「拒绝」也要报。
//
// 三种写法都算构造点（ADR-0048）：样本里复合字面量各一处，另加 `new` 与零值变量各一处。
func TestCheckFailure(t *testing.T) {
	m := loadFixture(t, "violations")
	vs := CheckFailure(m)
	if len(vs) != 4 {
		t.Fatalf("want 4 条（闸门造起不来两次、解析造拒绝两次），got %d：\n%s", len(vs), Report(vs))
	}
	if !anyContains(vs, "internal/gate/judge.go") || !anyContains(vs, "CannotStart") {
		t.Errorf("闸门里造「起不来」没报出来：\n%s", Report(vs))
	}
	if !anyContains(vs, "internal/parse/run.go") || !anyContains(vs, "Refusal") {
		t.Errorf("闸门外造「拒绝」没报出来：\n%s", Report(vs))
	}
	for _, want := range []string{"（new）", "（零值变量）"} {
		if !anyContains(vs, want) {
			t.Errorf("换个写法造同一个类型没报出来（缺 %q）：\n%s", want, Report(vs))
		}
	}
}

// 约定类型不见了也要报——否则删掉它就能让这一类闭嘴。
func TestCheckFailureDetectsMissingType(t *testing.T) {
	m := loadFixture(t, "clean")
	m.Failures = []FailureSite{{Pkg: "contract", Type: "NoSuchType"}}
	vs := CheckFailure(m)
	if len(vs) != 1 || !strings.Contains(vs[0].String(), "不见了") {
		t.Fatalf("want 一条「不见了」，got：%v", vs)
	}
}
