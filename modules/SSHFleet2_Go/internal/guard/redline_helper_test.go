package guard

import "testing"

// helper 替测试喊失败：登记表与代码对不上就红。这是第四种拆法——掏空 helper。
func TestCheckHelperAssertions(t *testing.T) {
	m := loadFixture(t, "helper")
	m.ModulePath = helperHomeModule // 登记表只对自己这个模块生效，样本借它的名字
	m.Redline = []RedLineEntry{{Test: "TestThing"}}

	swapHelpers(t, []HelperEntry{{"shout", 2}})
	if vs := CheckRedline(m); len(vs) != 0 {
		t.Fatalf("登记与代码一致，应当零违规，got：\n%s", Report(vs))
	}

	swapHelpers(t, nil)
	vs := CheckRedline(m)
	if len(vs) != 1 || !anyContains(vs, "shout") || !anyContains(vs, "没登记") {
		t.Fatalf("want 一条「shout 没登记」，got：\n%s", Report(vs))
	}

	// 掏空一句：次数从 2 变 1，报出前后两个数。
	swapHelpers(t, []HelperEntry{{"shout", 1}})
	vs = CheckRedline(m)
	if len(vs) != 1 || !anyContains(vs, "shout") || !anyContains(vs, "1 次") || !anyContains(vs, "2 次") {
		t.Fatalf("want 一条「shout 次数 1 变 2」，got：\n%s", Report(vs))
	}

	// 登记了却不在：helper 被删或改了名，但调它的测试还在。
	swapHelpers(t, []HelperEntry{{"shout", 2}, {"gone", 1}})
	vs = CheckRedline(m)
	if len(vs) != 1 || !anyContains(vs, "gone") || !anyContains(vs, "已经没有了") {
		t.Fatalf("want 一条「gone 已经没有了」，got：\n%s", Report(vs))
	}

	// 真模块上不许空转：清空登记表必须报出没登记的 helper。
	// 模块名独立写在下面（不取 helperHomeModule）：这个常量一写错，判据整段静默跳过，
	// 而真工程仍旧全绿——那正是这张表要防的假绿灯。
	real, err := Load(Target{
		Dir:        "../..",
		ModulePath: "sshfleet2",
		Rules:      Rules(),
		Failures:   DefaultFailures(),
		Redline:    RedLine(),
	})
	if err != nil {
		t.Fatalf("读真模块失败：%v", err)
	}
	swapHelpers(t, nil)
	if vs := CheckRedline(real); len(vs) == 0 {
		t.Fatal("登记表空转：真模块里替测试喊失败的 helper，一条都没被看见")
	}
}

// swapHelpers 换掉登记表，测试结束时换回来。
func swapHelpers(t *testing.T, entries []HelperEntry) {
	t.Helper()
	old := helperAsserts
	helperAsserts = entries
	t.Cleanup(func() { helperAsserts = old })
}
