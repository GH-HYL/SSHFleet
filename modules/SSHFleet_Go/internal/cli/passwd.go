package cli

import (
	"fmt"
	"strings"

	"sshfleet/internal/credential"
)

// --change-password（批量改密）的取值与互斥。
//
// 取值随 credential.encrypt 开关：`false` 时参数就是明文密码，`true` 时是密文文件的绝对路径。
// 与 `-f`/`-a` 的取值形态**不同**：那两个值可以"文件还是文本"自动判定，这里不行——
// 明文密码完全可能恰好等于一个存在的文件名，自动判定会把密码当文件读，报错指不到真原因。
//
// 它的值也不进路径参数循环（含空格不报错、不做 `\`→`/`）：它是密码，不是路径。

// checkPasswdChange 校验 --change-password 与它的互斥参数，并把明文新密码解到 a.NewPassword。
func checkPasswdChange(a *Args) error {
	if a.ChangePassword == "" {
		return nil
	}
	if err := checkPasswdChangeExclusive(a); err != nil {
		return err
	}
	plain, problems, err := credential.ReadCredential(a.ChangePassword, a.credentialEncrypted, true)
	if err != nil {
		return fmt.Errorf("--change-password 的值读不出新密码\n原因：%v", err)
	}
	if len(problems) > 0 {
		return fmt.Errorf("--change-password 的值有问题：%s", strings.Join(problems, "；"))
	}
	a.NewPassword = plain
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
	case a.NoBash:
		return fmt.Errorf("--change-password 不能和 --no-bash 一起用\n改密不执行任何命令，没有可施加的对象")
	case a.sudoFlag || a.noSudoFlag:
		return fmt.Errorf("--change-password 不能和 --sudo / --no-sudo 一起用\n改的是登录账号自己的密码，与执行身份无关")
	}
	return nil
}
