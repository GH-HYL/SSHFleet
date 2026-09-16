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

// 登录方式要如实反映实际走的那条路：私钥解析失败而退回密码时，
// 光看配置只会说「密钥/密码」——「密钥其实已经打不开了」这件事就此消失。
func TestAuthMethodDescReportsKeyFallback(t *testing.T) {
	c := NewClient(&Config{IP: "127.0.0.1", Port: 22, User: "u", KeyContent: "not-a-private-key", Password: "p"})
	if _, err := c.buildAuthMethods(); err != nil {
		t.Fatalf("配了密码时应回退密码认证，不该报错：%v", err)
	}
	if got := c.authMethodDesc(); got != "密码（密钥解析失败，已回退）" {
		t.Fatalf("登录方式应说明已回退，实际 %q", got)
	}

	// 只有密钥、解析失败：直接报错，登录方式仍是配置口径
	c2 := NewClient(&Config{IP: "127.0.0.1", Port: 22, User: "u", KeyContent: "bad"})
	if _, err := c2.buildAuthMethods(); err == nil {
		t.Fatal("只有密钥且解析失败时应报错")
	}
	if got := c2.authMethodDesc(); got != "密钥" {
		t.Fatalf("未回退时登录方式应为配置口径，实际 %q", got)
	}
}
