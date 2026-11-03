package parse

// `--change-password` 的取值：按加密开关解释成新密码，形态不对当场拦下。
//
// 取值随 credential.encrypt 开关：`false` 时这一格就是明文密码，`true` 时是密文文件的
// 绝对路径。与 `-f`／`-a` 的取值形态**不同**：那两个值可以「文件还是文本」自动判定，
// 这里不行——明文密码完全可能恰好等于一个存在的文件名，自动判定会把密码当文件读，
// 报错指不到真原因。
//
// **值一律不改写**（ADR-0027）：不 trim、不删换行。明文模式与解出来的明文都只在
// checkPasswdShape 里判形态，命中就报错退出。

import (
	"os"
	"strings"

	"sshfleet2/internal/contract"
	"sshfleet2/internal/credential"
	"sshfleet2/internal/message"
)

// passwdValue 把命令行给的那个值解释成新密码明文，装进纸上那一格。
//
// 四种失败各说各的下一步：贴错了东西、文件读不到、解不出来、内容不对。
// 前三样调用者要做的动作完全不同，所以不许混成一句「值有问题」。
func passwdValue(p *contract.Plan, encrypted bool) *contract.CannotStart {
	raw := p.Task.Passwd.Raw
	if raw == "" {
		return nil
	}

	// 密文只能装进文件、由路径指过来。有人会把密文那串本身贴进这一格：按路径去 Stat 会报
	// 「读不到：AgAA…」，看的人以为是文件名写错；开了加密又贴内容，连「该写路径」都说不清。
	if credential.IsEncrypted(raw) {
		if encrypted {
			return materialFail(message.PasswdValueIsCipherEncrypted())
		}
		return materialFail(message.PasswdValueIsCipherPlain())
	}

	// 打开加密时这个值是密文文件的路径：读不到就先给三行套，原因取系统原文——
	// 只说「值有问题」，看的人得自己猜是路径写错还是文件被挪了。
	if encrypted {
		if _, err := os.Stat(raw); err != nil {
			return materialFail(message.PasswdValueUnreadable(raw, message.SystemReason(err)))
		}
	}

	plain, problems, err := credential.PlainValue(raw, encrypted, true)
	if err != nil {
		return materialFail(message.PasswdValueUndecodable(err.Error()))
	}
	if len(problems) > 0 {
		return materialFail(message.PasswdValueProblems(joinProblems(problems)))
	}
	if fail := checkPasswdShape(plain); fail != nil {
		return fail
	}
	p.Task.Passwd.NewPassword = plain
	return nil
}

// joinProblems 把凭据包交回的那几条问题翻成一句：按老版的口径用「；」连起来。
func joinProblems(problems []credential.ValueProblem) string {
	parts := make([]string, 0, len(problems))
	for _, pr := range problems {
		parts = append(parts, message.PasswdProblemLabel(string(pr.Code), pr.Shown, pr.Detail))
	}
	return strings.Join(parts, "；")
}

// checkPasswdShape 判新密码的形态，命中就报错退出——**不改值**（ADR-0027）。
//
// 两条：任意位置有换行或回车（改密逐行喂进远端终端，换行等于回车，提交上去是半截密码）；
// 开头或结尾有空白（远端看不见，设上去以后自己也敲不出来）。中间的空格是正常写法，放行。
// 全是空白的值按第二条拦——它开头就是空白。
func checkPasswdShape(plain string) *contract.CannotStart {
	if strings.ContainsAny(plain, "\r\n") {
		return materialFail(message.PasswdHasNewline())
	}
	if strings.TrimSpace(plain) != plain {
		return materialFail(message.PasswdHasOuterSpace())
	}
	return nil
}
