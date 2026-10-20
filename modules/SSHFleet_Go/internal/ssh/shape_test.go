package ssh_test

// 结构形态 → 报文的导出纯函数。
//
// 这批用例是原先挂在判定包单测里那批「给一组布尔 → 期望一个中文分类名」表格的新家：
// 改造后判定层不再按结构字段起名，形态被说成一句陈述事实的报文，分类归属由判据表决定。
// 期望值一律按 spec《预算：全部报文清单》手写，不拿被测代码跑一遍抄下来。

import (
	"testing"

	"sshfleet/internal/ssh"
)

// —— transfer ——

// 回滚报文（ADR-0022）：上传失败后清掉本次写进去的文件，说清删了几个、还剩哪些。
func TestRollbackMessage(t *testing.T) {
	cases := []struct {
		name    string
		written int
		removed int
		left    []string
		want    string
	}{
		{
			name: "全清干净", written: 3, removed: 3, left: nil,
			want: "sshfleet.transfer.rolled_back: 3 of 3 written files removed",
		},
		{
			name: "只清掉一部分", written: 3, removed: 1, left: []string{"/data/a.bin", "/data/b.bin"},
			want: "sshfleet.transfer.rollback_partial: 1 of 3 written files removed, left: /data/a.bin, /data/b.bin",
		},
		{
			// 一个都没写成功时目标机本来就没留下东西，不必声明。
			name: "一个都没写进去", written: 0, removed: 0, left: nil,
			want: "",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ssh.RollbackMessage(c.written, c.removed, c.left); got != c.want {
				t.Fatalf("RollbackMessage = %q，期望 %q", got, c.want)
			}
		})
	}
}

// —— passwd ——

// passwdShape 改密收尾要问纯函数的那几个事实（与 Result 的字段一一对应，但不传整个结构）。
func passwdShape(fed, earlyClose, timedOut, sawEnter, hasReason bool) ssh.PasswdShape {
	return ssh.PasswdShape{
		FedAny:          fed,
		EarlyClose:      earlyClose,
		TimedOut:        timedOut,
		EnterSignalSeen: sawEnter,
		RemoteSaid:      hasReason,
	}
}

func TestPasswdStuckMessage(t *testing.T) {
	cases := []struct {
		name string
		in   ssh.PasswdShape
		want string
	}{
		{
			name: "喂过又超时",
			in:   passwdShape(true, false, true, true, false),
			want: "sshfleet.passwd.no_reply: password fed but no reply before timeout",
		},
		{
			name: "没喂过又超时",
			in:   passwdShape(false, false, true, false, false),
			want: "sshfleet.passwd.no_signal: no password prompt seen before timeout",
		},
		{
			name: "早收场：远端还在重问",
			in:   passwdShape(true, true, false, true, true),
			want: "sshfleet.passwd.give_up: no further prompts after answers used up",
		},
		{
			name: "非 0 收场、没喂过、远端一个字没说",
			in:   passwdShape(false, false, false, false, false),
			want: "sshfleet.passwd.no_prompt: passwd exited without asking for input",
		},
		{
			name: "非 0 收场、喂过、远端一个字没说",
			in:   passwdShape(true, false, false, true, false),
			want: "sshfleet.passwd.give_up: no further prompts after answers used up",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ssh.PasswdStuckMessage(c.in); got != c.want {
				t.Fatalf("PasswdStuckMessage = %q，期望 %q", got, c.want)
			}
		})
	}
}

func TestPasswdStuckMessageYieldsToRemote(t *testing.T) {
	// 远端说了原因就让位给判据表按原话认，不再补一句 passwd.*（与改造前的让位口径一致）。
	// 超时与早收场两条例外：那时远端那点内容多半是"命令执行超时"这类通用描述，
	// 照它分只会盖掉"改密卡在哪一步"这个更有用的信息。
	if got := ssh.PasswdStuckMessage(passwdShape(true, false, false, true, true)); got != "" {
		t.Fatalf("远端说了话不该再补报文，实际 %q", got)
	}
	if got := ssh.PasswdStuckMessage(passwdShape(false, false, false, true, true)); got != "" {
		t.Fatalf("远端说了话不该再补报文，实际 %q", got)
	}
}

func TestPasswdNotExpiredMessage(t *testing.T) {
	if got, want := ssh.PasswdNotExpiredMessage(false), "sshfleet.passwd.not_expired: no password-expiry prompt seen"; got != want {
		t.Fatalf("PasswdNotExpiredMessage = %q，期望 %q", got, want)
	}
	if got := ssh.PasswdNotExpiredMessage(true); got != "" {
		t.Fatalf("出现过期信号时不该说未过期，实际 %q", got)
	}
}

// —— answer ——

func TestAnswerMessage(t *testing.T) {
	cases := []struct {
		name                string
		missed              int
		exhausted, timedOut bool
		want                string
	}{
		{"两条没送出去", 2, false, false, "sshfleet.answer.missed: 2 answers not delivered"},
		{"一条没送出去（同时超时）", 1, false, true, "sshfleet.answer.missed: 1 answer not delivered"},
		{"全送出去了仍超时", 0, true, true, "sshfleet.answer.exhausted_timeout: all answers sent but no output before timeout"},
		{"全送出去了、正常收场", 0, true, false, ""},
		{"没配代填", 0, false, false, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ssh.AnswerMessage(c.missed, c.exhausted, c.timedOut); got != c.want {
				t.Fatalf("AnswerMessage = %q，期望 %q", got, c.want)
			}
		})
	}
}

// —— abort ——

func TestAbortMessage(t *testing.T) {
	if got, want := ssh.AbortMessage(true, true), "sshfleet.abort.canceled: interrupted after session established"; got != want {
		t.Fatalf("已连上被中断 = %q，期望 %q", got, want)
	}
	if got, want := ssh.AbortMessage(true, false), "sshfleet.abort.connect_canceled: interrupted while connecting"; got != want {
		t.Fatalf("未连上被中断 = %q，期望 %q", got, want)
	}
	if got := ssh.AbortMessage(false, true); got != "" {
		t.Fatalf("没被中断时不该发 abort 报文，实际 %q", got)
	}
}
