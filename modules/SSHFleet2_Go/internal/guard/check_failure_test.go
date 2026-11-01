package guard

import (
	"strings"
	"testing"
)

// 类五：闸门里造「起不来」要报；闸门外造「拒绝」也要报。
func TestCheckFailure(t *testing.T) {
	m := loadFixture(t, "violations")
	vs := CheckFailure(m)
	if len(vs) != 2 {
		t.Fatalf("want 2 条（闸门造起不来、解析造拒绝），got %d：\n%s", len(vs), Report(vs))
	}
	if !anyContains(vs, "internal/gate/judge.go") || !anyContains(vs, "CannotStart") {
		t.Errorf("闸门里造「起不来」没报出来：\n%s", Report(vs))
	}
	if !anyContains(vs, "internal/parse/run.go") || !anyContains(vs, "Refusal") {
		t.Errorf("闸门外造「拒绝」没报出来：\n%s", Report(vs))
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
