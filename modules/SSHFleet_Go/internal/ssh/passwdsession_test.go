package ssh

import "testing"

// passwdNote 三个兜底文案的分流：改密失败拿不到远端原因时，全靠这三句说清「错在哪种形状」。
// 分流错了会误导排查方向（2026-09-29 实测：没喂出去且超时被报成「提示没认出来」，
// 真实原因是改密模式漏了 -t 默认值，用户会去查提示词表，方向完全错）。
func TestPasswdNoteFallbacks(t *testing.T) {
	neverFed := newPasswdHook(&lockedBuffer{}, PasswdInput{Prompts: &PasswdPrompts{Steps: []PasswdStep{{Max: 1}}}}, "")

	if got := passwdNote("", neverFed, interactiveOutcome{timedOut: true}); got != passwdQuiet {
		t.Fatalf("没喂出去且到点应归「等到超时也没喂出一句」：%s", got)
	}
	if got := passwdNote("", neverFed, interactiveOutcome{}); got != passwdNoPrompt {
		t.Fatalf("没喂出去且未到点应归「提示没认出来」：%s", got)
	}

	fed := newPasswdHook(&lockedBuffer{}, PasswdInput{Prompts: &PasswdPrompts{Steps: []PasswdStep{{Max: 1}}}}, "")
	fed.fed = 1
	if got := passwdNote("passwd：密码未更改。", fed, interactiveOutcome{}); got != "passwd：密码未更改。" {
		t.Fatalf("远端说了原因应取原话：%s", got)
	}
	if got := passwdNote("", fed, interactiveOutcome{timedOut: true}); got != passwdStalled {
		t.Fatalf("喂出去后不吭声应归「远端没有回应」：%s", got)
	}
	if got := passwdNote("", fed, interactiveOutcome{aborted: true}); got != passwdGaveUp {
		t.Fatalf("反复重问应归「答案给完了」：%s", got)
	}
}
