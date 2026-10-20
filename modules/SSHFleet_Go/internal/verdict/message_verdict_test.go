package verdict

// 报文时代的判定行为（ADR-0021 决定第 3 / 8 条）：
//
//   - 判定层不再按结构字段给分类名——那 11 个中文常量退役；
//   - 结构事实由执行侧在收尾时说成一句报文（`sshfleet.<环境>.<环节>: …`），
//     与远端原话走同一套查表；
//   - 判定层读结构字段只为**成功规则**；
//   - 唯一不由查表产生的是 `执行失败(退出码N)`。
//
// 这批用例用**手写的报文文本**当输入，不经过执行侧——报文该怎么写由 ssh 包那批纯函数用例
// 守住，这里只守判定层的这一侧：拿到报文，查得出配置里那一类。

import (
	"strings"
	"testing"

	"sshfleet/internal/cli"
	"sshfleet/internal/ssh"
)

// judgeText 造一条「连上了、失败了」的结果，把报文放进报错原文那一格，跑一遍判定。
func judgeText(t *testing.T, kw *Keywords, mode cli.Mode, text string) ssh.Result {
	t.Helper()
	res := ssh.Result{ConnectSuccess: true, Error: strPtrOf(text)}
	Judge(&res, mode, kw)
	return res
}

// TestJudgeTakesCategoryFromMessage 报文当关键词用：被判据表认领，分类名来自配置。
func TestJudgeTakesCategoryFromMessage(t *testing.T) {
	kw := loadKeywords(t)

	cases := []struct {
		name   string
		text   string
		mode   cli.Mode
		expect string
	}{
		{
			name:   "建连被拒：原始错误原样跟在报文后面",
			text:   "sshfleet.connect.refused: dial tcp 10.0.0.5:22: connect: connection refused",
			mode:   cli.ModeCommand,
			expect: "拒绝网络连接",
		},
		{
			name:   "传输部分成功：由配置认领那句 finished",
			text:   "c.log: 下载失败 - permission denied\nsshfleet.transfer.finished: 3 of 5 files transferred",
			mode:   cli.ModeDownload,
			expect: "部分成功",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := judgeText(t, kw, c.mode, c.text).Category; got != c.expect {
				t.Fatalf("分类 = %q，期望 %q", got, c.expect)
			}
		})
	}
}

// TestJudgeUnclaimedMessageFallsBackToText 配置没认领的报文：分类回落成报文原文本身，
// 不吞、不猜（与今天「认不出来就显示报错原文」同一个位置）。
func TestJudgeUnclaimedMessageFallsBackToText(t *testing.T) {
	kw := loadKeywords(t)
	const text = "sshfleet.transfer.finished: 3 of 5 files transferred"
	// 传一条配置里没有的分类都不含的报文，专看好落点。
	const stray = "sshfleet.run.not_in_config: something happened"

	got := judgeText(t, kw, cli.ModeDownload, stray).Category
	if got != stray {
		t.Fatalf("未认领的报文应回落成原文，实为 %q", got)
	}
	_ = text
}

// TestJudgeSuccessRuleRejectsMessage 成功规则多一条：报错原文里出现报文即不算成功。
//
// 必须加：一级定性删掉之后，「代填没送完但命令退出 0」不再有一级分支抢在成功规则前面，
// 没有这一条就会被判成成功。
func TestJudgeSuccessRuleRejectsMessage(t *testing.T) {
	kw := loadKeywords(t)
	zero := 0
	res := ssh.Result{
		ConnectSuccess:  true,
		SessionBegun:    true,
		CommandExitCode: &zero,
		Error:           strPtrOf("sshfleet.answer.missed: 2 answers not delivered"),
	}
	Judge(&res, cli.ModeScript, kw)
	if res.Verdict == Success {
		t.Fatalf("报错原文里有报文时不该判成功：%+v", res)
	}
	if res.Category != "触发词未命中" {
		t.Fatalf("分类应来自判据表（触发词未命中），实为 %q", res.Category)
	}
}

// TestJudgeCleanSuccessUnaffected 报文规则不误伤干净的退出码 0。
func TestJudgeCleanSuccessUnaffected(t *testing.T) {
	kw := loadKeywords(t)
	zero := 0
	res := ssh.Result{ConnectSuccess: true, SessionBegun: true, CommandExitCode: &zero}
	Judge(&res, cli.ModeCommand, kw)
	if res.Verdict != Success {
		t.Fatalf("退出码 0、无超时无中断、原文里没有报文，应判成功：%+v", res)
	}
}

// TestJudgePasswordExpiredWithPasswdGuide 报文时代「密码过期」仍由远端原话认领。
func TestJudgePasswordExpiredWithPasswdGuide(t *testing.T) {
	kw := loadKeywords(t)
	one := 1

	// 第二组：命令以非 0 收场，输出里是 PAM 的改密对话
	res := ssh.Result{
		ConnectSuccess:  true,
		SessionBegun:    true,
		CommandExitCode: &one,
		Output:          "WARNING: Your password has expired.\nYou must change your password now and login again!",
	}
	Judge(&res, cli.ModeCommand, kw)
	if res.Category != "密码过期" {
		t.Fatalf("会话被拒应归「密码过期」，实为 %q", res.Category)
	}

	// 代填中止：中止词命中的原文并入待匹配文本，分类照常由判据表给出
	res = ssh.Result{
		ConnectSuccess: true,
		SessionBegun:   true,
		AbortLine:      "WARNING: Your password has expired.",
	}
	Judge(&res, cli.ModeCommand, kw)
	if res.Category != "密码过期" {
		t.Fatalf("中止词命中应归「密码过期」，实为 %q", res.Category)
	}
}

// TestJudgeNoCategoryConstants 判定结果里不许再出现代码给定的中文分类名：
// 判据表里把相关分类删干净之后，同一个结构形态只会回落成原文或「原因未知」。
func TestJudgeNoCategoryConstants(t *testing.T) {
	// 空判据表：任何形态都不可能命中分类。
	kw := &Keywords{}
	const msg = "sshfleet.transfer.finished: 3 of 5 files transferred"

	res := ssh.Result{ConnectSuccess: true, SuccessFiles: 3, FailedFiles: 2, Error: strPtrOf(msg)}
	Judge(&res, cli.ModeUpload, kw)
	if res.Category != msg {
		t.Fatalf("没有判据表时分类应回落成原文，实为 %q", res.Category)
	}

	// 取消两档也不能再靠结构字段起名
	canceled := "sshfleet.abort.canceled: interrupted after session established"
	res = ssh.Result{ConnectSuccess: true, SessionBegun: true, Canceled: true, Error: strPtrOf(canceled)}
	Judge(&res, cli.ModeCommand, kw)
	if res.Category != canceled {
		t.Fatalf("没有判据表时被取消应回落成原文，实为 %q", res.Category)
	}

	// 改密：结构字段齐全但配置没登记，同样回落
	res = ssh.Result{ConnectSuccess: true, SessionBegun: true, TimedOut: true, PasswdFedAny: false,
		Error: strPtrOf("sshfleet.passwd.no_signal: no password prompt seen before timeout")}
	Judge(&res, cli.ModePasswd, kw)
	if !strings.HasPrefix(res.Category, "sshfleet.passwd.no_signal") {
		t.Fatalf("没有判据表时改密卡住应回落成原文，实为 %q", res.Category)
	}
}
