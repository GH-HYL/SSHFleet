package guard

import "testing"

// 类四：契约包里出现函数就报。
func TestCheckContract(t *testing.T) {
	wantOne(t, CheckContract(loadFixture(t, "violations")), ClassContract,
		"internal/contract/pure.go", "契约包里声明了函数 Leak")

	if vs := CheckContract(loadFixture(t, "clean")); len(vs) != 0 {
		t.Errorf("clean 样本的契约包只有类型，应当零违规，got：%v", vs)
	}
}
