package ssh

// 执行一摊的报文：会话与终端的三种建不起来、命令到点、命令被拒。
//
// 重点守两件事：
//   - 同一情形只有一个环节名——三条通道（命令 / 代填 / 改密）撞上同一个错误时，
//     产出的报文必须逐字相同，不能各写一套；
//   - 库已经分好类的错误不许丢（ExitMissingError 以前是被直接丢掉的）。

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"
)

// 三条通道共用的会话建不起来：同一个底层错误，必须产出同一句报文。
func TestSessionFailureMessageIsSharedAcrossChannels(t *testing.T) {
	raw := "ssh: rejected: administratively prohibited (open failed)"
	want := "sshfleet.session.create_failed: " + raw
	if got := Message("session", sessionCreateFailed, raw); got != want {
		t.Fatalf("报文 = %q，期望 %q", got, want)
	}
}

// 命令被目标机拒绝（Start 返回 OpenChannelError）：这是一个独立的环节，
// 不能被当成普通错误原文、也不该靠 `ssh: command * failed` 这种库措辞去认。
func TestEndFactsTagsRejectedCommand(t *testing.T) {
	err := &ssh.OpenChannelError{
		Reason:  ssh.Prohibited,
		Message: "administratively prohibited",
	}
	f := endFactsOf(err, false, false)
	if f.exitCode != nil {
		t.Fatalf("命令没跑起来不该有退出码，实为 %v", *f.exitCode)
	}
	if f.errText == nil {
		t.Fatal("应当记下原文")
	}
	if got := *f.errText; !strings.HasPrefix(got, "sshfleet.run.rejected: ") {
		t.Fatalf("命令被拒应发 run.rejected，实为 %q", got)
	}
	// 原始信息一个不吞：库的 reason 与 message 都要在冒号后面看得到
	if !strings.Contains(*f.errText, "administratively prohibited") {
		t.Fatalf("原始错误被吞掉了：%q", *f.errText)
	}
}

// 会话结束了、没收到退出状态：库已经分好类，翻成报文，不许退化成靠关键词认库的提示原文。
func TestEndFactsTagsExitMissing(t *testing.T) {
	err := &ssh.ExitMissingError{}
	f := endFactsOf(err, false, false)
	if f.errText == nil || !strings.HasPrefix(*f.errText, "sshfleet.session.exit_missing: ") {
		t.Fatalf("应发 session.exit_missing，实为 %v", f.errText)
	}
}

// 认不出退出码的普通错误照旧记原文，不被硬套上报文。
func TestEndFactsKeepsPlainErrorText(t *testing.T) {
	f := endFactsOf(fmt.Errorf("wrap: %w", errors.New("boom")), false, false)
	if f.errText == nil {
		t.Fatal("认不出退出码时应当记原文")
	}
	if strings.HasPrefix(*f.errText, "sshfleet.") {
		t.Fatalf("普通错误不该被硬套报文：%q", *f.errText)
	}
}

// 超时：只能说一次，且只能有一个环节名（命令通道与代填通道各一处，产出必须相同）。
func TestRunTimeoutMessage(t *testing.T) {
	if got, want := Message("run", runTimeout, "exceeded 30s"), "sshfleet.run.timeout: exceeded 30s"; got != want {
		t.Fatalf("报文 = %q，期望 %q", got, want)
	}
}
