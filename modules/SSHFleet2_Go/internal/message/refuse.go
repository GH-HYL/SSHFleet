package message

import (
	"fmt"
	"strings"

	"sshfleet2/internal/contract"
)

// 闸门那几类拒绝的话：闸门只交事实（哪一条没过、哪个选项、哪段原文），句子在这里成形。
//
// 判据是 ADR-0034 决定八（出判断的地方不写话）与 ADR-0028 决定一、五（标签与新行只许在
// 构造点加）。每个函数返回的都是**套好外壳、分好行**的正文，闸门拿到就交给 `Fatal`，一个字
// 都不再添。
//
// 文案逐字照老工程：`internal/cli/check.go` 的 CheckArguments 与 `internal/cli/args.go` 的
// 解析失败那两句。这里是使用者看得见的面，改一个字都是改行为。

// ConfigShownPath 是面向使用者的配置文件路径：不带 `./`，它不是路径的一部分。
//
// 它出现在给使用者看的话里（例如「请改 xxx 的 [interpreter]」），所以归这里；
// 解析站要展示同一处路径时也向这里要，两处不许各写一份。
const ConfigShownPath = "config/SSHFleet.conf"

// 三句外壳，对位老版 main.go 的两处 wrapFatal 与危险规则表那一处。
//
// 它们是**话**，所以只住这里：解析站造「起不来」时向本包要，别处不许各写一份
// （ADR-0028 决定一）。
const (
	HeadParse       = "选项/参数解析失败"
	HeadCompliance  = "选项/参数合规性检查未通过"
	HeadDangerRules = "危险关键词内容检查未通过"
)

// Shell 给正文套上老版那层壳：`<外壳>：<正文>`。head 为空表示那一处没有外壳句，正文原样交回。
func Shell(head, body string) FatalBody {
	if head == "" {
		return bodyOf(body)
	}
	return bodyOf(head + "：" + body)
}

// helpHint 是两条词法错共用的收尾。
const helpHint = "（使用 -h 查看帮助）"

// modeMutual 是模式零个与多个共用的问题句：两次都错在同一件事上。
const modeMutual = "执行模式选项：-c、-s、-u、-d、--change-password 互斥，只能指定一个"

// RefuseUnknownOption 表里没有这个选项（name 带着横杠，例如 `--zzz`）。
func RefuseUnknownOption(name string) FatalBody {
	return Shell(HeadParse, "不认识的选项："+name+helpHint)
}

// RefuseMissingValue 带值的选项后面没有值（name 带着横杠，例如 `-c`）。
func RefuseMissingValue(name string) FatalBody {
	return Shell(HeadParse, "选项 "+name+" 缺少值"+helpHint)
}

// RefuseExtraArgs 多出来的位置参数：本机收到的比想要的多，说明原文没被完整收下。
//
// owner 是「第一个多余段前面那个选项名」（解析站回溯出来的事实，认不出为空）；
// joined 是多余段按原样拼起来。三条常见原因与那句提示逐字照老版 `args.go` 的
// `extraArgsError`——这几句是给使用者排错用的，改一个字都是改行为。
//
// 这一段不用 `ProblemHint`：它的中间还夹着三条编号原因，版式与「问题／提示」两段不同。
// 编号与那个缩进是屏幕上的样子，所以只许在这里成形。
func RefuseExtraArgs(owner, joined string) FatalBody {
	head := "出现多余的参数（未被使用）：" + joined
	if owner != "" {
		// 认得出主：多半是值里有空格、没加引号，被 shell 切成了两段。
		head = owner + " 的值后面还有一段没法识别：" + joined
	}
	return Shell(HeadParse, head+"\n常见原因：\n"+
		"  ① 值里有空格但没加引号——shell 按空格把参数切成了两段\n"+
		"  ② 通配符被终端展开（`-u /x/abc/*` 会变成多个路径；上传目录不需要通配符）\n"+
		"  ③ 参数打多了或打错了"+
		hintLabel+"本来想写成一个值的，把整条用双引号包起来，例如 -p \"/opt/my dir/\"（bash、Git Bash、cmd、PowerShell 都认双引号）")
}

// 三个数值选项在报错里怎么称呼自己。解析站只交类别（`contract.ProblemSubject`），
// 称呼是话，在这里成形——不归站。
func subjectText(s contract.ProblemSubject) string {
	switch s {
	case contract.SubjectConcurrency:
		return "并发连接数"
	case contract.SubjectTimeout:
		return "命令或传输超时时间"
	case contract.SubjectConnect:
		return "连接超时时间"
	}
	panic("拒绝：取值主语 " + string(s) + " 没有称呼")
}

// RefuseBadPositiveInt 三个数值选项（-n／-t／-T）取值认不出或不是正数。
// subject 是这一格怎么称呼自己（上面那三个常量之一）。
func RefuseBadPositiveInt(flag string, subject contract.ProblemSubject, raw string) FatalBody {
	return Shell(HeadCompliance, ProblemHint(
		CurrentValue(flag+" 参数格式错误，"+subjectText(subject)+"必须是正整数", raw),
		"给一个正整数（单位见该选项的说明）"))
}

// RefuseModeNone 一个模式都没给。老版把「零个」与「多个」分开说：零个要做的是选一个，
// 不是删掉多余的。
func RefuseModeNone() FatalBody {
	return Shell(HeadCompliance, ProblemHint(
		modeMutual,
		"你没有指定任何模式，请从下面五个里选一个：\n"+
			"      -c 执行命令   -s 执行脚本   -u 上传文件   -d 下载文件   --change-password 批量改密"))
}

// RefuseModeMultiple 同时给了多个模式。
func RefuseModeMultiple() FatalBody {
	return Shell(HeadCompliance, ProblemHint(
		modeMutual,
		"一次只能做一件事，请只保留一个模式"))
}

// RefuseNoShellWithOtherMode `--no-shell` 只和 `-c` 一起用。
func RefuseNoShellWithOtherMode() FatalBody {
	return Shell(HeadCompliance, ProblemHint(
		"--no-shell 只能和 -c 一起使用",
		"请去掉 --no-shell，或改用 -c 执行"))
}

// RefusePasswdWithAnswer 改密不能配 -a。
func RefusePasswdWithAnswer() FatalBody {
	return Shell(HeadCompliance, ProblemHint(
		"--change-password 不能和 -a 一起用",
		"改密不用你配触发词，用的是一份统一的提示词表"))
}

// RefusePasswdWithPath 改密不能配 -p。
func RefusePasswdWithPath() FatalBody {
	return Shell(HeadCompliance, ProblemHint(
		"--change-password 不能和 -p 一起用",
		"改密不传文件，用不到目标路径"))
}

// RefusePasswdWithKey 改密不能配 -k。
func RefusePasswdWithKey() FatalBody {
	return Shell(HeadCompliance, ProblemHint(
		"--change-password 不能和 -k 一起用",
		"改密要用账号自己的登录密码登进去，密钥登录派不上用场"))
}

// RefuseRemotePathNotAbsolute 远端路径（上传目录、下载源）不以 / 开头。
func RefuseRemotePathNotAbsolute(flag, what, value string) FatalBody {
	return Shell(HeadCompliance, ProblemHint(
		CurrentValue(flag+" 参数指定的"+what+"必须是绝对路径", value),
		"服务器上的路径要从 / 写起，例如 /opt/app/"))
}

// RefuseRemotePathHasNewline 远端路径里有回车或换行：参数被换行打断的现场。
func RefuseRemotePathHasNewline(flag, what, value string) FatalBody {
	return Shell(HeadCompliance, ProblemHint(
		flag+" 参数指定的"+what+"里有回车或换行："+value,
		"一条命令只写一个路径，检查复制时是不是把换行带进来了"))
}

// RefuseUploadDirNeedsSlash 上传的 -p 必须以 / 收尾（ADR-0002 的消歧义约束）。
func RefuseUploadDirNeedsSlash(value string) FatalBody {
	return Shell(HeadCompliance, ProblemHint(
		CurrentValue("上传模式：-p 必须是以 / 结尾的目录", value),
		"-p 只表示\"放到哪个目录\"，工具不会替你改文件名，所以要以 / 收尾"))
}

// RefusePathWithCommand `-p` 不能和 `-c`／`-s` 一起用。
func RefusePathWithCommand() FatalBody {
	return Shell(HeadCompliance, ProblemHint(
		"-p 参数不能搭配 -c 或 -s 使用",
		"只有上传、下载才用 -p；命令与脚本模式不需要它"))
}

// RefuseDownloadNeedsPath `-d` 必须配 `-p`。
func RefuseDownloadNeedsPath() FatalBody {
	return Shell(HeadCompliance, ProblemHint(
		"-d 参数必须搭配 -p 参数使用",
		"-p 写文件要存到本机的哪个目录，例如 -p ./logs/"))
}

// RefuseUploadNeedsPath `-u` 必须配 `-p`。
func RefuseUploadNeedsPath() FatalBody {
	return Shell(HeadCompliance, ProblemHint(
		"-u 参数必须搭配 -p 参数使用",
		"-p 写文件要放到服务器上的哪个目录，例如 -p /opt/app/"))
}

// RefuseSudoConflict 两个执行身份开关同给：自相矛盾，只给一行、不补提示（老版如此）。
func RefuseSudoConflict() FatalBody {
	return Shell(HeadCompliance, "--sudo 与 --no-sudo 只能给一个（两个都不给=按配置里的值）")
}

// RefuseCommandEmpty `-c` 给了但只有空白。
func RefuseCommandEmpty() FatalBody {
	return Shell(HeadCompliance, ProblemHint(
		"-c 参数不能为空，请提供要执行的命令",
		"-c 后面跟命令原文，例如 -c \"systemctl status sshd\""))
}

// RefusePasswdEmpty `--change-password` 给了但只有空白。
func RefusePasswdEmpty() FatalBody {
	return Shell(HeadCompliance, ProblemHint(
		"--change-password 的新密码不能为空",
		"后面跟这次要统一改成的新密码"))
}

// —— 阶段 05 的五条互斥、两条 `-a` 判据 ——
//
// 文案逐字照老版 `cli/check.go` 的 CheckArguments 与 `cli/answer.go` 的 checkAnswer*：
// 哪一句带「提示：」、哪一句另起一行不带上标签，都照原样——使用者照着做的那一步不一样。

// RefuseAnswerWithUpload `-a` 不能和 `-u` 一起用。
func RefuseAnswerWithUpload() FatalBody {
	return Shell(HeadCompliance, ProblemHint(
		"-a 不能和 -u 一起用",
		"代填是给命令、脚本的交互用的，上传时没有命令在跑"))
}

// RefuseAnswerWithDownload `-a` 不能和 `-d` 一起用。
func RefuseAnswerWithDownload() FatalBody {
	return Shell(HeadCompliance, ProblemHint(
		"-a 不能和 -d 一起用",
		"代填是给命令、脚本的交互用的，下载时没有命令在跑"))
}

// RefuseAnswerWithNoShell `--no-shell` 不能和 `-a` 一起用。
func RefuseAnswerWithNoShell() FatalBody {
	return Shell(HeadCompliance, ProblemHint(
		"--no-shell 不能和 -a 一起使用",
		"请去掉其中一个"))
}

// RefusePasswdWithSudo 改密不能和身份开关一起用：第二行不带上标签（老版就那样）。
func RefusePasswdWithSudo() FatalBody {
	return Shell(HeadCompliance, "--change-password 不能和 --sudo / --no-sudo 一起用\n"+
		"改的是登录账号自己的密码，与执行身份无关")
}

// RefuseAnswerTooLong `-a` 下发的命令太长：量的是**整条下发行**，不是正文。
//
// body 是正文的字节数（老版那一格还在，是要让人知道限的是哪一头），line 是量出来的
// 下发行字节数，limit 是写的上限。
func RefuseAnswerTooLong(body, line, limit int) FatalBody {
	return Shell(HeadCompliance, fmt.Sprintf(
		"-a 参数下发的命令太长：正文 %d 字节，下发 %d 字节，上限 %d 字节\n"+
			"提示：请把正文控制在 7 万字节以内（代填要把正文编进命令行，长度受它限制）",
		body, line, limit))
}

// RefuseAnswerUnsupportedInterpreter `-a` 不支持这个解释器：表外的名字没有统一的
// 「一段程序文本」入口，硬跑只会静默不跑（perl 用 -c 甚至只做语法检查）。
func RefuseAnswerUnsupportedInterpreter(interpreter string) FatalBody {
	return Shell(HeadCompliance, fmt.Sprintf(
		"-a 代填不支持解释器 %q\n"+
			"目前支持：%s\n"+
			"提示：请改 %s 的 [interpreter]，换成上面这些；或去掉 -a 后重跑",
		interpreter, strings.Join(SupportedInterpreters(), "、"), ConfigShownPath))
}
