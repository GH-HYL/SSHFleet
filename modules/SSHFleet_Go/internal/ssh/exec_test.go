package ssh

import "testing"

// Output 字段的整备回归（用户 2026-09-15 裁定的分层）：只在采集侧做一次——
// 去掉整块**最前 / 最后**的空白行；中间的空行与每行的行首缩进都是输出自身的结构，
// 一个字都不动（终端与 output.txt 拿它直接输出，落 xlsx 时只额外清非法字符）。

func TestTrimOuterBlankLines(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"去首尾空行", "\n\na\nb\n\n", "a\nb"},
		{"去首尾纯空白行", "   \n\ta\nb\t\n  \n", "\ta\nb\t"},
		{"保留行首缩进", "         system boot  2026-09-15 10:23", "         system boot  2026-09-15 10:23"},
		{"保留中间空行", "a\n\nb", "a\n\nb"},
		{"整块都是空白", "\n\n   \n", ""},
		{"无输出", "", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := trimOuterBlankLines(c.in); got != c.want {
				t.Fatalf("trimOuterBlankLines(%q) 应为 %q，实际 %q", c.in, c.want, got)
			}
		})
	}
}
