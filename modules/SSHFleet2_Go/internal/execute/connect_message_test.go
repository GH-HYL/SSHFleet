package execute

// 建连那一摊的翻译：原始错误的文本 → 环节名 → 报文。
//
// 期望值一律手写字面量，不拿被测函数算一遍（规范三·3）。

import (
	"context"
	"errors"
	"testing"
)

func TestConnectStageOfDialError(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want string
	}{
		{"端口没开", errors.New("dial tcp 127.0.0.1:2201: connect: connection refused"), connectRefused},
		{"Windows 的说法", errors.New("No connection could be made because the target machine actively refused it."), connectRefused},
		{"连接被重置", errors.New("read tcp 10.0.0.1:22: connection reset by peer"), connectRefused},
		{"路由不可达", errors.New("dial tcp 10.0.0.9:22: connect: no route to host"), connectNoRoute},
		{"网络不可达", errors.New("dial tcp 10.0.0.9:22: connect: network is unreachable"), connectNoRoute},
		{"到点", errors.New("dial tcp 10.0.0.9:22: i/o timeout"), connectTimeout},
		{"泛指的超时", errors.New("context deadline exceeded (Client.Timeout exceeded while awaiting headers)"), connectTimeout},
		{"被取消归通用拨号失败", context.Canceled, connectDialFailed},
		{"包着的取消也归通用拨号失败", &wrapped{inner: context.Canceled}, connectDialFailed},
		{"认不出的原文", errors.New("some vendor specific failure"), connectDialFailed},
		{"没有错误", nil, connectDialFailed},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := connectStageOfDialError(c.err); got != c.want {
				t.Fatalf("环节名应为 %q，实际 %q", c.want, got)
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
		{"密码错", errors.New("ssh: handshake failed: ssh: unable to authenticate, attempted methods [none password], no supported methods remain"), connectAuthFailed},
		{"只剩一句", errors.New("no supported methods remain"), connectAuthFailed},
		{"账号过期那种 EOF", errors.New("ssh: handshake failed: EOF"), connectHandshakeFailed},
		{"认不出的原文", errors.New("ssh: overflow reading version string"), connectHandshakeFailed},
		{"没有错误", nil, connectHandshakeFailed},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := connectStageOfHandshakeError(c.err); got != c.want {
				t.Fatalf("环节名应为 %q，实际 %q", c.want, got)
			}
		})
	}
}

func TestConnectMessageKeepsRawText(t *testing.T) {
	got := connectMessage(connectRefused, errors.New("dial tcp 127.0.0.1:2201: connect: connection refused"))
	want := "sshfleet.connect.refused: dial tcp 127.0.0.1:2201: connect: connection refused"
	if got != want {
		t.Fatalf("报文应为\n%q\n实际\n%q", want, got)
	}

	got = connectMessage(connectHandshakeFailed, nil)
	want = "sshfleet.connect.handshake_failed: "
	if got != want {
		t.Fatalf("没有原文时报文应为 %q，实际 %q", want, got)
	}
}

// TestMsgMarksUnknownEnv 环境名写错时，报文形状要一眼看得出坏：环境段后面多一个 `!`。
func TestMsgMarksUnknownEnv(t *testing.T) {
	if got, want := msg("transf", "rolled_back", "x"), "sshfleet.transf!.rolled_back: x"; got != want {
		t.Fatalf("未知环境名应拼出坏形状 %q，实际 %q", want, got)
	}
	if got, want := msg(envConnect, "refused", "x"), "sshfleet.connect.refused: x"; got != want {
		t.Fatalf("已知环境名应为 %q，实际 %q", want, got)
	}
}

// wrapped 包一层错误，用来验 errors.Is 认得出包在里面的取消。
type wrapped struct{ inner error }

func (w *wrapped) Error() string { return "包一层：" + w.inner.Error() }
func (w *wrapped) Unwrap() error { return w.inner }
