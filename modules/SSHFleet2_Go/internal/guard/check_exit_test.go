package guard

import "testing"

// 类三：塔尖之外调 os.Exit 要报；塔尖里调不报。
func TestCheckExit(t *testing.T) {
	wantOne(t, CheckExit(loadFixture(t, "violations")), ClassExit, "internal/parse/run.go", "os.Exit")

	if vs := CheckExit(loadFixture(t, "clean")); len(vs) != 0 {
		t.Errorf("clean 样本里 os.Exit 只在塔尖，应当零违规，got：%v", vs)
	}
}
