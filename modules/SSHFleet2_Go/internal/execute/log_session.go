package execute

// 执行期日志的开头一段：这一轮打算怎么做、发下去的是什么。
//
// 版式逐字照老版：`batch/batch.go` 的两行（执行参数、开始执行任务）与
// `ssh/command.go` 的 `DescribeCommand`（下发行那一块）。
//
// 为什么要有这一段：命令行里只剩固定形态的 `sh -c`，事后翻日志看不出「原始命令是什么、
// 被包成了哪一行、用哪个身份跑的」。这两行与那一块就是这一轮的现场——给维护者看，不是
// 给使用者的版面（`docs/个人开发规范.md` 四.4：记录保留现场，不包装）。

import (
	"fmt"
	"strings"

	"sshfleet2/internal/contract"
	"sshfleet2/internal/log"
	"sshfleet2/internal/message"
)

// logSessionHead 写开头两行，再交代下发行。
//
// limit 是**实际生效的并发数**（0 已经在调用方换成了节点数）——老版记的也是生效值，
// 不是命令行上的原值：事后看日志要能直接答「这次几台一起跑」。
//
// 「执行超时」这个措辞五种模式都这么写（老版也这样），只是值按模式取：上传下载记的是
// 传输超时那一格。三处（确认屏、报告、这里）取值都走 `message.TimeoutSeconds`。
func logSessionHead(lg *log.Logger, p *contract.Plan, limit int) {
	if lg == nil {
		return
	}
	// 上传源里被过滤的链接排在最前：它说的是「这次少传了什么」，属于开跑前要交代的事实，
	// 老版也记在这两行之前。
	if skipped := uploadSkipped(p); len(skipped) > 0 {
		lg.Info(message.UploadSkippedText(skipped))
	}
	lg.Info(fmt.Sprintf("执行参数：执行超时 %d 秒，连接超时 %d 秒，执行身份 %s",
		message.TimeoutSeconds(p.Mode, p.Run.TimeoutExecute, p.Run.TimeoutTransfer),
		p.Run.TimeoutConnect, identityLabel(p.Run)))
	lg.Info(fmt.Sprintf("开始执行任务：节点 %d 个，并发 %d，模式 %s",
		len(p.Nodes), limit, message.LogName(p.Mode)))

	for _, line := range sendBlock(p) {
		lg.Success(line)
	}
}

// uploadSkipped 上传源里被过滤掉的链接；不是上传模式就是空。
func uploadSkipped(p *contract.Plan) []string {
	if p.Task.Upload == nil {
		return nil
	}
	return p.Task.Upload.Skipped
}

// sendBlock 交代下发给远端的是什么。四种形态，各是各自的实际形态，不合并：
//
//	命令模式            → 命令行 / stdin / 说明
//	脚本模式            → 命令行 / stdin（脚本体不进日志，只说明是哪份）/ 说明
//	命令或脚本 + 代填   → 代填表 + 原始命令（脚本）/ 处理方式 / 最终命令
//	上传、下载、改密    → 没有「交给谁解释」这件事，不记这一块
func sendBlock(p *contract.Plan) []string {
	if answers := answersOf(p); len(answers) > 0 {
		return answersBlock(p, answers)
	}
	if p.Task.Script != nil {
		return scriptBlock(p)
	}
	if p.Task.Command != nil {
		return downBlock(p)
	}
	return nil
}

// answersBlock 代填那一支：先把表摆出来（谁答、答什么），再交代发下去的是什么。
//
// 这一支不写「命令行 / stdin」：代填时 stdin 整条让给了代填，命令行里也没有正文——
// 正文被 base64 编进去了，所以记「原始命令（脚本）」与「最终命令」两头。
func answersBlock(p *contract.Plan, answers []contract.Answer) []string {
	lines := []string{fmt.Sprintf("代填表（%d 条）：", len(answers))}
	for i, a := range answers {
		lines = append(lines, fmt.Sprintf("  第 %d 条：%s", i+1, message.AnswerDescribe(a)))
	}

	var text string
	if p.Task.Script != nil {
		text = "交给 SSH 执行：\n" +
			"  原始脚本： " + p.Task.Script.Path + "（正文经 base64 编入命令行，不落盘）\n" +
			fmt.Sprintf("  处理方式： 会话 stdin 整条让给代填（%d 条）\n", len(answers)) +
			"  最终命令： " + p.Run.Downlink
	} else {
		text = "交给 SSH 执行：\n" +
			"  原始命令： " + p.Task.Command.Text + "\n" +
			fmt.Sprintf("  处理方式： 正文经 base64 编入命令行（不落盘），会话 stdin 整条让给代填（%d 条）\n", len(answers)) +
			"  最终命令： " + p.Run.Downlink
	}
	return append(lines, indentLines(text)...)
}

// scriptBlock 脚本模式（无代填）：正文走 stdin，所以 stdin 那一行只说是哪份脚本，
// 不把正文贴进日志。
func scriptBlock(p *contract.Plan) []string {
	text := "交给 SSH 执行：\n" +
		"  命令行： " + p.Run.Downlink + "\n" +
		"  stdin：  脚本 " + p.Task.Script.Path + " 的内容（" + p.Run.Interpreter + " 解释）\n" +
		"  说明：   " + envNote(p.Run) + scriptIdentity(p.Run) + "执行 stdin 送来的脚本"
	return indentLines(text)
}

// scriptIdentity 脚本模式「说明」里那个身份：老版写「以 root 身份」或「以登录用户身份」。
func scriptIdentity(run contract.RunParams) string {
	if run.Sudo {
		return "以 root 身份"
	}
	return "以登录用户身份"
}

// identityLabel 日志里「执行身份」那一格：要了 root 且没被 --no-shell 削掉才算 root。
//
// 取的是**实际生效**的那一个，不是使用者要的那一个——`--sudo --no-shell` 下两者不同，
// 这里必须记前者（老版 `Args.Identity()` 同一条）。
func identityLabel(run contract.RunParams) string {
	if run.Sudo && !run.NoShell {
		return "root"
	}
	return "登录用户"
}

// downBlock 交代下发行：命令行、stdin、说明三行，整块缩进两格自成一段。
//
// 三行各自说的是实际发生的事，不描述包装过程（没有「原始／最终」这种前后对照）。
// 说明那一句只讲做了什么：先导入 `[env]`（配了才提）、再判断目标机编码、语言开关开了
// 才提「钉成英文」，最后说这条命令交给谁执行。
func downBlock(p *contract.Plan) []string {
	command, stdin := p.Run.Downlink, p.Run.Stdin

	if p.Run.NoShell {
		return indentLines("交给 SSH 执行：\n" +
			"  命令行： " + command + "\n" +
			"  说明：   --no-shell 原样执行，不套外壳、不经 stdin")
	}

	return indentLines("交给 SSH 执行：\n" +
		"  命令行： " + command + "\n" +
		"  stdin：  " + stdin + "\n" +
		"  说明：   " + envNote(p.Run) + "把 stdin 送来的命令" + commandWho(p.Run.Interpreter, p.Run.Sudo))
}

// indent 日志正文逐行缩进两格（说明块在日志里自成一段）。
func indentLines(text string) []string {
	lines := make([]string, 0, 4)
	for _, ln := range strings.Split(text, "\n") {
		lines = append(lines, "  "+ln)
	}
	return lines
}

// envNote 「说明」里交代开头那半句。
//
// 空格只留在中文与西文交界处（「的 bash」「bash 执行」），中文之间不加——老版为这一处
// 订正过一次（ADR-0028 决定五的来历）。
func envNote(run contract.RunParams) string {
	what := "判断目标机编码"
	if run.English {
		what = "判断目标机编码、把提示语钉成英文"
	}
	if len(run.Env) > 0 {
		return "先导入配置文件里的环境变量，再" + what + "，然后"
	}
	return "先" + what + "，然后"
}

// commandWho 「说明」行的谓语：这条命令交给谁执行。
func commandWho(interpreter string, asRoot bool) string {
	if asRoot {
		return "交给 root 身份的 " + interpreter + "（sudo）执行"
	}
	return "交给登录用户的 " + interpreter + " 执行"
}
