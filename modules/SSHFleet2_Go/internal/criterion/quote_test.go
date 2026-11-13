package criterion

import "testing"

// 三形状各钉一次：转义规则改一个字，下发串就拼坏，所以期望值全是写死的字面量。

func TestEscapeSingle(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"", ""},
		{"abc", "abc"},
		{"a'b", `a'\''b`},
		{"''", `'\'''\''`},
		{`"x"`, `"x"`}, // 双引号不归这条管
	} {
		if got := EscapeSingle(c.in); got != c.want {
			t.Errorf("EscapeSingle(%q) = %q，想要 %q", c.in, got, c.want)
		}
	}
}

func TestSingleQuoted(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"", "''"},
		{"a", "'a'"},
		{"a'b", `'a'\''b'`},
		{"a\nb", "'a\nb'"},   // 换行原样包进去：整段仍是一个参数
		{"$HOME", "'$HOME'"}, // 下发侧要它原样交给目标节点展开
	} {
		if got := SingleQuoted(c.in); got != c.want {
			t.Errorf("SingleQuoted(%q) = %q，想要 %q", c.in, got, c.want)
		}
	}
}

func TestQuoteForShell(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"", "''"},
		{"-f", "-f"},
		{"10.0.0.1,22,root", "10.0.0.1,22,root"}, // 逗号安全：内联清单加了引号反而失真
		{"who -b", "'who -b'"},                   // 空白要引号，否则照抄时被拆成两个参数
		{"a'b", `'a'\''b'`},
		{"$HOME", "'$HOME'"}, // 显示侧要拦住展开
	} {
		if got := QuoteForShell(c.in); got != c.want {
			t.Errorf("QuoteForShell(%q) = %q，想要 %q", c.in, got, c.want)
		}
	}
}
