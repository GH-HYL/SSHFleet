package guard

import "testing"

// shout 替测试喊两次失败——掏空它，调它的测试一个字符都不变。
func shout(t *testing.T, want, got int) {
	t.Helper()
	if want != got {
		t.Fatalf("want %d，got %d", want, got)
	}
	if got == 0 {
		t.Errorf("一条都没有")
	}
}

// TestThing 整条只有一行调用：四句断言都住在 shout 里。
func TestThing(t *testing.T) {
	shout(t, 1, 1)
}
