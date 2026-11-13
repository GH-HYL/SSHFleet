package message

import (
	"strconv"
	"strings"

	"sshfleet2/internal/contract"
	"sshfleet2/internal/criterion"
)

// 确认屏：按确认之前，把这次要做什么摊开给人看。版式逐字照老版 confirm 那一屏——
// 使用者按下回车之前看的就是它，差一个空格都会被当成「工具改版了」。
//
// 事实从纸上来（模式、命令、清单、台数、并发、超时、备注），排版只在这一处成形。

// modeRowLabel 是「执行模式」这一格的标签：确认屏的表头与 `report.txt` 的同一行都用它。
//
// 一处成形——两处各写一次，改叫法就要改两处，而它们印的是同一件事。
const modeRowLabel = "执行模式"

// 那一屏用到的颜色（老版 common/color.go 的取值，一个不改）。
const (
	ansiBrightCyan   = "\x1b[96m"
	ansiBrightOrange = "\x1b[38;5;214m"
	ansiBold         = "\x1b[1m"
	ansiBrightYellow = "\x1b[93m"
)

// confirmTitle 是横幅里那行字。前后各留的空格与老版一致，边框长度按它的显示宽度算。
const confirmTitle = "           SSHFleet - 执行选项/参数确认           "

// ConfirmScreen 是确认那一屏：横幅 + 一张两列表。
//
// manifest 与 maskedPassword 都是**已经遮过**的：内联清单里就写着密码、新密码更是凭据本身，
// 都不能原样上屏。遮法住 mask，由调用方（闸门）取一次递进来——`message` 的依赖白名单里
// 只有契约，不新开一条叶子之间的边。
func ConfirmScreen(p *contract.Plan, manifest, maskedPassword string, concurrencyAdopted bool) Line {
	border := strings.Repeat("═", len([]rune(confirmTitle))+10)

	var b strings.Builder
	b.WriteString("\n" + ansiCyan + "╔" + border + "╗" + ansiReset + "\n")
	b.WriteString(ansiCyan + "║  " + confirmTitle + "  ║" + ansiReset + "\n")
	b.WriteString(ansiCyan + "╚" + border + "╝" + ansiReset + "\n\n")
	b.WriteString(infoTable(confirmRows(p, manifest, maskedPassword, concurrencyAdopted)))
	return Line{s: b.String()}
}

// confirmRows 是那张表的行，次序照老版：模式与内容 → 清单与台数 → 执行参数。
//
// 空的一行（两列都空）在排版里落成一个换行，用来把表分块。
func confirmRows(p *contract.Plan, manifest, maskedPassword string, concurrencyAdopted bool) [][2]string {
	var t [][2]string

	// 执行身份行只在上传／下载／改密出现：命令与脚本的身份要么与登录用户一致、要么被
	// --no-shell 剥掉，不生效的身份摆一行没有信息量（老版 2026-10-28 定）。
	switch p.Mode {
	case contract.ModeUpload, contract.ModeDownload, contract.ModePasswd:
		t = append(t, [2]string{"执行身份", IdentityLabel(p.Run)})
	}

	// 解释器行只在命令／脚本模式出现，且只在解释器不是 bash 时占一行——默认值不必提醒。
	// 这一格问的是「是不是那个默认值 bash」，不是「是不是 shell 家族」——后者是解析站查表的事
	// （`sh`、`dash` 也算 shell 家族，但它们不是默认值，仍要占一行）。
	// --no-shell 时解释器配置不参与执行，这一行改放提示。
	switch p.Mode {
	case contract.ModeCommand, contract.ModeScript:
		if p.Run.Interpreter != "" && criterion.InterpreterBaseName(p.Run.Interpreter) != "bash" {
			value := p.Run.Interpreter
			if p.Run.NoShell {
				value = "原样下发，本次不生效"
			}
			t = append(t, [2]string{"解释器", value})
		}
	}

	switch p.Mode {
	case contract.ModeCommand:
		// 标签用「命令内容」不用「执行命令」：-c 收的是一段命令文本，后者两种读法都通，
		// 把文件名误敲给 -c 时扫一眼就过去了（老版 2026-10 定的措辞）。
		t = append(t, [2]string{modeRowLabel, modeTitle(p.Mode)}, [2]string{"命令内容", p.Task.Command.Text}, [2]string{"", ""})
	case contract.ModeScript:
		t = append(t, [2]string{modeRowLabel, modeTitle(p.Mode)}, [2]string{"脚本路径", p.Task.Script.Path}, [2]string{"", ""})
	case contract.ModeUpload:
		t = append(t, [2]string{modeRowLabel, modeTitle(p.Mode)},
			[2]string{"本地路径", p.Task.Upload.Source},
			[2]string{"远程路径", p.Task.Path},
			[2]string{"", ""})
	case contract.ModeDownload:
		t = append(t, [2]string{modeRowLabel, modeTitle(p.Mode)},
			[2]string{"远程路径", p.Task.Download.RemotePath},
			[2]string{"本地路径", p.Task.Path},
			[2]string{"", ""})
	case contract.ModePasswd:
		// 新密码是明确知道身份的凭据：按下确认之前看得见形态，但只看得到脱敏形态。
		// 取解析后那份明文去遮（加密模式下原值是一份密文文件的路径，遮出来是一串路径，
		// 屏上核不到这次到底要设什么）。
		t = append(t, [2]string{modeRowLabel, modeTitle(p.Mode)},
			[2]string{"新密码", maskedPassword})
	}

	t = append(t, answerRows(p)...)
	t = append(t,
		[2]string{"节点清单", manifest},
		[2]string{"节点数量", strconv.Itoa(len(p.Nodes))},
		[2]string{"并发数", concurrencyText(p, concurrencyAdopted)},
		[2]string{"", ""},
	)
	if p.Run.TimeoutConnect != 0 {
		t = append(t, [2]string{"连接超时", strconv.Itoa(p.Run.TimeoutConnect) + "s"})
	}
	// 值按模式取：上传下载印的是传输超时那一格，命令/脚本/改密印执行超时那一格。
	if secs := TimeoutSeconds(p.Mode, p.Run.TimeoutExecute, p.Run.TimeoutTransfer); secs != 0 {
		t = append(t, [2]string{timeoutLabel(p.Mode), strconv.Itoa(secs) + "s"})
	}
	if p.Run.Remark != "" {
		t = append(t, [2]string{"备注", strings.TrimSpace(p.Run.Remark)})
	}
	return t
}

// answerRows 参数屏里的代填行：每条一行，含内容与它的全部触发词（顺序同命令行）。
//
// 代填会直接改远端行为，按下确认之前得看得见这次配了什么。
func answerRows(p *contract.Plan) [][2]string {
	var answers []contract.Answer
	switch {
	case p.Task.Command != nil:
		answers = p.Task.Command.Answers
	case p.Task.Script != nil:
		answers = p.Task.Script.Answers
	}
	rows := make([][2]string, 0, len(answers))
	for i, entry := range answers {
		rows = append(rows, [2]string{"代填 " + strconv.Itoa(i+1), AnswerDescribe(entry)})
	}
	return rows
}

// IdentityLabel 「执行身份」那一格的值：取**实际生效**的那一个，不是使用者要的那一个
// （`--sudo --no-shell` 下两者不同，记的必须是前者，老版 `Args.Identity()` 同一条）。
//
// 确认屏那一行与执行期日志那一格是同一个判断，只在这里成形；老版也只有一处。
func IdentityLabel(run contract.RunParams) string {
	if run.Sudo && !run.NoShell {
		return "root"
	}
	return "登录用户"
}

// concurrencyText 是并发数那一格：没指定 -n 时补一句「全部并行」，让人知道这个数从哪来。
//
// 老版在出屏之前就把 args.Number 填成节点数，所以屏上写的是个数、外加一句解释。
func concurrencyText(p *contract.Plan, adopted bool) string {
	v := concurrencyValue(p.Run.Concurrency, len(p.Nodes))
	// 采纳了并发建议：这一行加粗并注明，别让人以为原值本来就是这个数（ADR-0040 决定三）。
	if adopted {
		return ansiBold + ansiBrightOrange + strconv.Itoa(v) + "（已按建议修改）" + ansiReset
	}
	return strconv.Itoa(v) + suffixForUnset(p.Run.Concurrency)
}

// concurrencyValue 是**实际生效**的并发数：没指定 -n（0）就是全部并行＝节点数；
// 指定了但比台数还多，也只能起这么多（2026-11-10，审计 05-F11）。
//
// 确认屏、`report.txt`、执行期日志三处读同一个换法。改前这里只换 0、不封顶，于是
// `-n 100` 配三台时屏上印 100、报告印 5、实际起 5——同一次运行两个并发数。
func concurrencyValue(n, total int) int {
	if n == 0 || n > total {
		return total
	}
	return n
}

// suffixForUnset 没指定 -n 时补的那半句；指定了就是空串。
func suffixForUnset(n int) string {
	if n != 0 {
		return ""
	}
	return "（全部并行）"
}

// infoTable 把两列表排成对齐的文本：标签按显示宽度补齐，值一律亮橙。
//
// 对齐用显示宽度而不是字符数——「并发数」三个汉字占六列，按 rune 数补会错位。
func infoTable(rows [][2]string) string {
	maxLabel := 0
	for _, r := range rows {
		if w := DisplayWidth(r[0]); w > maxLabel {
			maxLabel = w
		}
	}

	var b strings.Builder
	for _, r := range rows {
		if r[0] == "" && r[1] == "" {
			b.WriteString("\n")
			continue
		}
		label := r[0] + strings.Repeat(" ", maxLabel-DisplayWidth(r[0]))
		// 值默认套橙色；自带样式的（以加粗开头，如「已按建议修改」那一行）原样输出，
		// 免得外面再套一层——两层叠起来转义序列就与老版对不上。
		value := ansiBrightOrange + r[1] + ansiReset
		if strings.HasPrefix(r[1], ansiBold) {
			value = r[1]
		}
		b.WriteString(ansiBrightCyan + "▶ " + label + "-→" + ansiReset + "   " + value + "\n")
	}
	return b.String()
}

// ExecConfirmPrompt 是确认屏之后那句问句：先一条 60 个等号的分隔线，再问一句。
//
// 方括号与冒号由 ask 补（它按缺省是是还是否决定写 [Y/n] 还是 [y/N]），这里只给问句。
func ExecConfirmPrompt() Line {
	s := "\n" + strings.Repeat("═", 60) + "\n\n" +
		ansiBrightYellow + "以上确认无误，开始执行？" + ansiReset
	return Line{s: s}
}

// ConfirmSkippedLine 是非交互（--yes）跳过确认时上屏的那一行。
//
// 老版此时**不打参数屏**，只打这一行——`--yes` 是替人回答了「y」，不是替人看了屏。
func ConfirmSkippedLine() Line {
	return Line{s: ansiYellow + " [非交互模式] 跳过执行选项/参数确认环节，直接执行" + ansiReset + "\n\n"}
}

// ConfirmSkippedTrace 是跳过确认写进工具日志的那一句。
func ConfirmSkippedTrace() Line {
	return Line{s: "非交互模式（--yes）跳过执行选项/参数确认，直接执行"}
}

// StartLine 是用户按下 y 之后那两行「开始执行」。
func StartLine() Line {
	return Line{s: "SSHFleet工具" + "\x1b[34m" + "开始执行" + ansiReset + "......\n" +
		strings.Repeat("=", 50) + "\n"}
}

// CancelTrace 是确认屏上答否时写进工具日志的那一句（老版 logger.Warn 的原话）。
func CancelTrace() Line {
	return Line{s: "执行已取消，SSHFleet工具已退出"}
}

// refuseEnvelope 是「标签与载荷对不上」那一句的外壳。
//
// 正常用不到：它只在纸被造错时才出现（写着命令、装的却是脚本）。真要撞上就是内部缺陷，
// 所以话说清是「这次要做什么与装的东西不一致」，不往使用者身上推。
//
// 括号里那截由下面三个具名构造点给：闸门只交事实（有哪几份、标签是什么、装的是什么），
// 句子在这里成形（ADR-0034 决定八）。
func refuseEnvelope(what string) FatalBody {
	return bodyOf(HeadCompliance + "：这次要做什么与它的载荷对不上（" + what + "）")
}

// RefuseEnvelopeMulti 一次装了不止一份载荷：labels 是那几份的称呼。
func RefuseEnvelopeMulti(labels []string) FatalBody {
	return refuseEnvelope("同时装了" + strings.Join(labels, "、"))
}

// RefuseEnvelopeEmpty 标签说是哪一种活，载荷却是空的。
func RefuseEnvelopeEmpty(want string) FatalBody {
	return refuseEnvelope("标签是" + want + "，载荷却是空的")
}

// RefuseEnvelopeWrong 标签与载荷对不上：标签是 want，装的是 got。
func RefuseEnvelopeWrong(want, got string) FatalBody {
	return refuseEnvelope("标签是" + want + "，装的是" + got)
}
