package cli

import (
	"fmt"
	"os"
	"strings"

	"sshfleet/internal/common"
	"sshfleet/internal/credential"
)

// --change-password（批量改密）的取值与互斥。
//
// 取值随 credential.encrypt 开关：`false` 时参数就是明文密码，`true` 时是密文文件的绝对路径。
// 与 `-f`/`-a` 的取值形态**不同**：那两个值可以"文件还是文本"自动判定，这里不行——
// 明文密码完全可能恰好等于一个存在的文件名，自动判定会把密码当文件读，报错指不到真原因。
//
// 它的值也不进路径参数循环（含空格不报错、不做 `\`→`/`）：它是密码，不是路径。
//
// **值一律不改写**（ADR-0027）：不 trim、不删换行。加密模式那份路径原样交 OS；
// 明文模式与解出来的明文都只在 checkNewPasswordShape 里判形态，命中就报错退出。

// checkPasswdChange 校验 --change-password 与它的互斥参数，并把明文新密码解到 a.NewPassword。
func checkPasswdChange(a *Args) error {
	if a.ChangePasswordRaw == "" {
		return nil
	}
	if err := checkPasswdChangeExclusive(a); err != nil {
		return err
	}
	// 打开加密时这个值是密文文件的路径：读不到就先给三行套（问题 / 原因 / 提示），
	// 原因取系统原文——只说「值有问题」，看的人得自己猜是路径写错还是文件被挪了。
	if a.credentialEncrypted {
		if _, err := os.Stat(a.ChangePasswordRaw); err != nil {
			return common.ErrUser(
				fmt.Sprintf("--change-password 的值读不到：%s", a.ChangePasswordRaw),
				common.SystemReason(err),
				"打开加密时这里写密文文件的本机绝对路径（用 --convert-secret 转出来的那个文件）")
		}
	}
	plain, problems, err := credential.ReadCredential(a.ChangePasswordRaw, a.credentialEncrypted, true)
	if err != nil {
		return common.ErrUserReason("--change-password 的值读不出新密码", err.Error())
	}
	if len(problems) > 0 {
		return fmt.Errorf("--change-password 的值有问题：%s", strings.Join(problems, "；"))
	}
	// 明文模式的原值与加密模式解出来的明文都走到这里，形态检查只此一处（ADR-0027）。
	if err := checkNewPasswordShape(plain); err != nil {
		return err
	}
	a.NewPassword = plain
	return nil
}

// checkNewPasswordShape 判新密码的形态，命中就报错退出——**不改值**（ADR-0027）。
//
// 两条：任意位置有换行或回车（改密逐行喂进远端终端，换行等于回车，提交上去是半截密码）；
// 开头或结尾有空白（远端看不见，设上去以后自己也敲不出来）。中间的空格是正常写法，放行。
// 全是空白的值按第二条拦——它开头就是空白。
func checkNewPasswordShape(plain string) error {
	if strings.ContainsAny(plain, "\r\n") {
		return common.ErrUser("--change-password 的新密码里有换行符或回车", "参数被换行截断，或复制粘贴时带进了换行", "把整串写成一行（或换一个密码）后重跑")
	}
	if strings.TrimSpace(plain) != plain {
		return common.ErrUser("--change-password 的新密码开头或结尾有空白", "多半是多打或复制粘贴时带进来的空格、制表符", "换一个不带首尾空白的密码后重跑；远端看不见这些字符，设上去以后你自己也敲不出来")
	}
	return nil
}

// checkPasswdChangeExclusive 改密与几个开关互斥：冲突的是"这次运行该长什么样"，
// 明确报错并说明原因，不静默忽略。
//
// 与 `-c`/`-s`/`-u`/`-d` 的互斥由 CheckArguments 的模式计数统一拦（五选一），不在这里重说。
func checkPasswdChangeExclusive(a *Args) error {
	switch {
	case a.Answer != "":
		return fmt.Errorf("--change-password 不能和 -a 一起用\n提示：改密不用你配触发词，用的是一份统一的提示词表")
	case a.Path != "":
		return fmt.Errorf("--change-password 不能和 -p 一起用\n提示：改密不传文件，用不到目标路径")
	case a.Key:
		return fmt.Errorf("--change-password 不能和 -k 一起用\n提示：改密要用账号自己的登录密码登进去，密钥登录派不上用场")
	case a.NoShell:
		return fmt.Errorf("--change-password 不能和 --no-shell 一起用\n改密不执行任何命令，没有可施加的对象")
	case a.sudoFlag || a.noSudoFlag:
		return fmt.Errorf("--change-password 不能和 --sudo / --no-sudo 一起用\n改的是登录账号自己的密码，与执行身份无关")
	}
	return nil
}
