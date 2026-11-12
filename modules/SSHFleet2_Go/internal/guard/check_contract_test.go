package guard

import "testing"

// 类四：契约包里出现函数就报——写成 func 声明、函数字面量、还是取一个函数名都算。
func TestCheckContract(t *testing.T) {
	vs := inClass(CheckContract(loadFixture(t, "violations")), ClassContract)
	if len(vs) != 3 {
		t.Fatalf("want 3 条（函数声明、函数字面量、取函数名），got %d：\n%s", len(vs), Report(vs))
	}
	for _, want := range []string{"函数 Leak", "装进了变量 LeakValue", "装进了变量 LeakRef"} {
		if !anyContains(vs, want) {
			t.Errorf("少了这一条：%q，实得：\n%s", want, Report(vs))
		}
	}
	for _, v := range vs {
		if v.File != "internal/contract/pure.go" {
			t.Errorf("位置不对：%s", v)
		}
	}

	if vs := CheckContract(loadFixture(t, "clean")); len(vs) != 0 {
		t.Errorf("clean 样本的契约包只有类型，应当零违规，got：%v", vs)
	}
}
