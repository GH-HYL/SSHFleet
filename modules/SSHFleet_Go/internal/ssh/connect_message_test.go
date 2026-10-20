package ssh

// 建连一摊的报文：把 Go / 系统的原始错误**无损翻译**成 `sshfleet.connect.*`，
// 原文原样跟在冒号后面（ADR-0021 决定第 1、6、8 条）。
//
// 这里测的是「哪种错误翻成哪个环节名」——判定层与配置都靠这个名字，
// 名字写错（哪怕只差一个下划线）那一类失败就会落到「未归类」。

import (
	"context"
	"errors"
	"net"
	"testing"
)

func TestConnectStageOfDialError(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want string
	}{
		{
			name: "端口没开",
			err:  &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("connect: connection refused")},
			want: "refused",
		},
		{
			name: "拨号超时",
			err:  errors.New("dial tcp 10.0.0.5:22: i/o timeout"),
			want: "timeout",
		},
		{
			name: "路由不可达",
			err:  errors.New("dial tcp 10.0.0.5:22: connect: no route to host"),
			want: "no_route",
		},
		{
			name: "域名解析不出来",
			err:  errors.New("dial tcp: lookup nope.example: no such host"),
			want: "dns_failed",
		},
		{
			name: "认不出的拨号错误：退化为通用环节名",
			err:  errors.New("dial tcp: something odd happened"),
			want: "dial_failed",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := connectStageOfDialError(c.err); got != c.want {
				t.Fatalf("环节名 = %q，期望 %q", got, c.want)
			}
		})
	}
}

func TestConnectStageOfHandshakeError(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want string
	}{
		{
			name: "认证被拒",
			err:  errors.New("ssh: handshake failed: ssh: unable to authenticate, attempted methods [none password], no supported methods remain"),
			want: "auth_failed",
		},
		{
			name: "认证阶段被对端掐断（只有 EOF）",
			err:  errors.New("ssh: handshake failed: EOF"),
			want: "handshake_failed",
		},
		{
			name: "不是 SSH 服务",
			err:  errors.New("ssh: handshake failed: ssh: overflow reading version string"),
			want: "handshake_failed",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := connectStageOfHandshakeError(c.err); got != c.want {
				t.Fatalf("环节名 = %q，期望 %q", got, c.want)
			}
		})
	}
}

// 联系上下文的取消不算目标机的毛病：它自己的报文在 abort 那一摊，
// 这里不能把它认成「端口没开」。
func TestCanceledDialIsNotARefusal(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := &net.OpError{Op: "dial", Net: "tcp", Err: ctx.Err()}
	if got := connectStageOfDialError(err); got == connectRefused {
		t.Fatalf("取消不该被认成拒绝连接")
	}
}

// 报文里的原始错误必须原样保留：前缀是我们的，冒号后面那段是别人的。
func TestConnectMessageKeepsRawError(t *testing.T) {
	raw := "dial tcp 10.0.0.5:22: connect: connection refused"
	got := connectMessage(connectRefused, errors.New(raw))
	if want := "sshfleet.connect.refused: " + raw; got != want {
		t.Fatalf("报文 = %q，期望 %q", got, want)
	}
}
