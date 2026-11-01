package guard

import "testing"

// 名单与代码一致时不报；指纹改窄、测试没了，各报一条。
func TestCheckRedline(t *testing.T) {
	good := []RedLineEntry{
		{Test: "TestCheck", Symbols: []string{"Check"}},
		{Test: "TestTwoPins", Symbols: []string{"gate.Refusal", "parse.Run"}},
	}

	m := loadFixture(t, "testsite")
	m.Redline = good
	if vs := CheckRedline(m); len(vs) != 0 {
		t.Fatalf("名单与代码一致，应当零违规，got：\n%s", Report(vs))
	}

	narrowed := []RedLineEntry{
		{Test: "TestCheck", Symbols: []string{}},
		{Test: "TestTwoPins", Symbols: []string{"gate.Refusal", "parse.Run"}},
	}
	vs := CheckRedline(withRedline(m, narrowed))
	if len(vs) != 1 || !anyContains(vs, "指纹对不上") {
		t.Fatalf("want 一条「指纹对不上」，got：%v", vs)
	}

	missing := []RedLineEntry{
		{Test: "TestCheck", Symbols: []string{"Check"}},
		{Test: "TestTwoPins", Symbols: []string{"gate.Refusal", "parse.Run"}},
		{Test: "TestGone", Symbols: nil},
	}
	vs = CheckRedline(withRedline(m, missing))
	if len(vs) != 1 || !anyContains(vs, "已经没有了") {
		t.Fatalf("want 一条「已经没有了」，got：%v", vs)
	}
}

func withRedline(m *Module, entries []RedLineEntry) *Module {
	out := *m
	out.Redline = entries
	return &out
}
