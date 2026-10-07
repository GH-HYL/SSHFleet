package ssh

import (
	"context"
	"testing"
	"time"
)

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

// 密钥解析失败就是失败——不回退密码：配了密钥却在背后偷偷用密码登录，
// 会让人以为密钥是好的，直到密码也失效那天才发现。
func TestBuildAuthMethodsNoPasswordFallback(t *testing.T) {
	c := NewClient(&Config{IP: "127.0.0.1", Port: 22, User: "u", KeyContent: "not-a-private-key", Password: "p"})
	if _, err := c.buildAuthMethods(); err == nil {
		t.Fatal("私钥解析失败时应直接报错，不该退回密码认证")
	}
	// 登录方式按配置口径如实报（密钥），不再有"已回退"这种说法
	if got := c.authMethodDesc(); got != "密钥" {
		t.Fatalf("登录方式应为配置口径，实际 %q", got)
	}
}

// 建连被外部中断时，结果里必须留下「取消」这个事实——判定侧靠它把"被中断"与"连不上"
// 分开报，而不是去辨认报错文案（用户 2026-10-08 真机：连接阶段被 Ctrl+C 带走的机器
// 报的是 operation was canceled，不在关键词表里，结果每台一个分类把汇总刷屏）。
func TestConnectCanceledIsRecorded(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 先取消：拨号会立刻以「operation was canceled」收场

	c := NewClient(&Config{IP: "192.0.2.1", Port: 22, User: "u", Password: "p", ConnectTimeout: time.Second})
	res := c.newResult(0)
	if c.connectFor(ctx, res) {
		t.Fatal("ctx 已取消时不该连上")
	}
	if !res.Canceled {
		t.Fatalf("建连被中断应记下 Canceled，实际 %+v", res)
	}
	if res.ConnectSuccess {
		t.Fatal("ConnectSuccess 应为 false")
	}
}

// 对照：没被中断的普通建连失败，不许多出一个 Canceled——否则「连不上」会被误报成「取消」。
func TestConnectFailureIsNotCanceled(t *testing.T) {
	c := NewClient(&Config{
		IP: "192.0.2.1", Port: 22, User: "u", Password: "p",
		ConnectTimeout: 200 * time.Millisecond, // TEST-NET 地址，必然连不上
	})
	res := c.newResult(0)
	if c.connectFor(context.Background(), res) {
		t.Fatal("不该连上")
	}
	if res.Canceled {
		t.Fatalf("普通建连失败不该被记成取消，实际 %+v", res)
	}
}
