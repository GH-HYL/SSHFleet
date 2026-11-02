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
func logSessionHead(lg *log.Logger, p *contract.Plan, limit int) {
	if lg == nil {
		return
	}
	lg.Info(fmt.Sprintf("执行参数：执行超时 %d 秒，连接超时 %d 秒，执行身份 %s",
		p.Run.TimeoutExecute, p.Run.TimeoutConnect, identityLabel(p.Run)))
	lg.Info(fmt.Sprintf("开始执行任务：节点 %d 个，并发 %d，模式 %s",
		len(p.Nodes), limit, message.LogName(p.Mode)))

	// 上传下载与改密没有「下发给谁解释」这件事，那几支随阶段 05。
	if p.Task.Command == nil {
		return
	}
	for _, line := range downBlock(p) {
		lg.Success(line)
	}
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
	command, stdin := downCommand(*p.Task.Command, p.Run)

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
