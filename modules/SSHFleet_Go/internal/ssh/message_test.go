package ssh_test

// 出厂报文（`sshfleet.<环境>.<环节>: <英文描述>`）的格式与外层形状。
//
// 这批用例只断言**报文长什么样**：前缀、环境段、环节段的写法。它们不拿被测代码产出的
// 字符串当作"正确值"的来源——期望值全部按 ADR-0021「决定」第 1 条手写的字面量。

import (
	"regexp"
	"strings"
	"testing"

	"sshfleet/internal/ssh"
)

// messageShape 报文的形状：`sshfleet.` + 环境 + `.` + 环节 + `: ` + 一句英文。
// 环境与环节都只由小写字母、数字、下划线组成（ADR-0021 第 1 条）。
var messageShape = regexp.MustCompile(`^sshfleet\.[a-z][a-z0-9_]*\.[a-z][a-z0-9_]*: \S`)

func TestMessageFollowsDeclaredShape(t *testing.T) {
	got := ssh.Message("connect", "refused", "dial tcp 1.2.3.4:22: connect: connection refused")
	if !messageShape.MatchString(got) {
		t.Fatalf("报文形状不对：%q", got)
	}
	const want = "sshfleet.connect.refused: dial tcp 1.2.3.4:22: connect: connection refused"
	if got != want {
		t.Fatalf("报文 = %q，期望 %q", got, want)
	}
}

func TestMessageKeepsOriginalErrorAfterColon(t *testing.T) {
	// 原始错误原样跟在冒号后面：不吞、不改、不加引号（ADR-0021 决定第 6 条）。
	const raw = `ssh: handshake failed: EOF`
	if got, want := ssh.Message("connect", "handshake_failed", raw), "sshfleet.connect.handshake_failed: ssh: handshake failed: EOF"; got != want {
		t.Fatalf("报文 = %q，期望 %q", got, want)
	}
}

func TestMessageRejectsUnknownEnvironment(t *testing.T) {
	// 环境取值只有七种；写错的环境名会让使用者无法按前缀认领整摊，
	// 宁可在产出的这一刻就暴露成可见的坏形状，也不静默放行。
	got := ssh.Message("transf", "finished", "1 of 2 files transferred")
	if messageShape.MatchString(got) {
		t.Fatalf("不认识的环境名不该产出合法报文：%q", got)
	}
	if !strings.HasPrefix(got, "sshfleet.") {
		t.Fatalf("无论环境名对错，前缀都该在：%q", got)
	}
}

func TestIsMessage(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want bool
	}{
		{"报文", "sshfleet.connect.refused: connection refused", true},
		{"原文在前报文在后的一格", "a.txt: 下载失败 - permission denied\nsshfleet.transfer.finished: 1 of 3 files transferred", true},
		{"远端原话不是报文", "Permission denied (publickey)", false},
		{"工具自编中文也不是报文", "建立连接失败 - dial tcp: connection refused", false},
		{"空串", "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ssh.IsMessage(c.in); got != c.want {
				t.Fatalf("IsMessage(%q) = %v，期望 %v", c.in, got, c.want)
			}
		})
	}
}
