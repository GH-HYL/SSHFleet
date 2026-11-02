package message

import (
	"strconv"
	"strings"

	"sshfleet2/internal/contract"
)

// 确认屏：按确认之前，把这次要做什么摊开给人看。版式逐字照老版 confirm 那一屏——
// 使用者按下回车之前看的就是它，差一个空格都会被当成「工具改版了」。
//
// 事实从纸上来（模式、命令、清单、台数、并发、超时、备注），排版只在这一处成形。

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
// manifest 是**已经遮过**的节点清单：内联清单里就写着密码，不能原样上屏。遮法住 mask，
// 由调用方（闸门）取一次递进来——`message` 的依赖白名单里只有契约，不新开一条叶子之间的边。
func ConfirmScreen(p *contract.Plan, manifest string) contract.Text {
	border := strings.Repeat("═", len([]rune(confirmTitle))+10)

	var b strings.Builder
	b.WriteString("\n" + ansiCyan + "╔" + border + "╗" + ansiReset + "\n")
	b.WriteString(ansiCyan + "║  " + confirmTitle + "  ║" + ansiReset + "\n")
	b.WriteString(ansiCyan + "╚" + border + "╝" + ansiReset + "\n\n")
	b.WriteString(infoTable(confirmRows(p, manifest)))
	return contract.Text{S: b.String()}
}

// confirmRows 是那张表的行，次序照老版：模式与内容 → 清单与台数 → 执行参数。
//
// 空的一行（两列都空）在排版里落成一个换行，用来把表分块。
func confirmRows(p *contract.Plan, manifest string) [][2]string {
	var t [][2]string

	switch p.Mode {
	case contract.ModeCommand:
		// 标签用「命令内容」不用「执行命令」：-c 收的是一段命令文本，后者两种读法都通，
		// 把文件名误敲给 -c 时扫一眼就过去了（老版 2026-10 定的措辞）。
		t = append(t, [2]string{"执行模式", "命令模式"}, [2]string{"命令内容", p.Task.Command.Text}, [2]string{"", ""})
	case contract.ModeScript:
		t = append(t, [2]string{"执行模式", "脚本模式"}, [2]string{"脚本路径", p.Task.Script.Path}, [2]string{"", ""})
	}

	t = append(t,
		[2]string{"节点清单", manifest},
		[2]string{"节点数量", strconv.Itoa(len(p.Nodes))},
		[2]string{"并发数", concurrencyText(p)},
		[2]string{"", ""},
	)
	if p.Run.TimeoutConnect != 0 {
		t = append(t, [2]string{"连接超时", strconv.Itoa(p.Run.TimeoutConnect) + "s"})
	}
	if p.Run.TimeoutExecute != 0 {
		t = append(t, [2]string{"执行超时", strconv.Itoa(p.Run.TimeoutExecute) + "s"})
	}
	if p.Run.Remark != "" {
		t = append(t, [2]string{"备注", strings.TrimSpace(p.Run.Remark)})
	}
	return t
}

// concurrencyText 是并发数那一格：没指定 -n 时补一句「全部并行」，让人知道这个数从哪来。
//
// 老版在出屏之前就把 args.Number 填成节点数，所以屏上写的是个数、外加一句解释。
func concurrencyText(p *contract.Plan) string {
	if p.Run.Concurrency != 0 {
		return strconv.Itoa(p.Run.Concurrency)
	}
	return strconv.Itoa(len(p.Nodes)) + "（全部并行）"
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
		b.WriteString(ansiBrightCyan + "▶ " + label + "-→" + ansiReset + "   " +
			ansiBrightOrange + r[1] + ansiReset + "\n")
	}
	return b.String()
}

// ExecConfirmPrompt 是确认屏之后那句问句：先一条 60 个等号的分隔线，再问一句。
//
// 方括号与冒号由 ask 补（它按缺省是是还是否决定写 [Y/n] 还是 [y/N]），这里只给问句。
func ExecConfirmPrompt() contract.Text {
	s := "\n" + strings.Repeat("═", 60) + "\n\n" +
		ansiBrightYellow + "以上确认无误，开始执行？" + ansiReset
	return contract.Text{S: s}
}

// ConfirmSkippedLine 是非交互（--yes）跳过确认时上屏的那一行。
//
// 老版此时**不打参数屏**，只打这一行——`--yes` 是替人回答了「y」，不是替人看了屏。
func ConfirmSkippedLine() contract.Text {
	return contract.Text{S: ansiYellow + " [非交互模式] 跳过执行选项/参数确认环节，直接执行" + ansiReset + "\n\n"}
}

// ConfirmSkippedTrace 是跳过确认写进工具日志的那一句。
func ConfirmSkippedTrace() contract.Text {
	return contract.Text{S: "非交互模式（--yes）跳过执行选项/参数确认，直接执行"}
}

// StartLine 是用户按下 y 之后那两行「开始执行」。
func StartLine() contract.Text {
	return contract.Text{S: "SSHFleet工具" + "\x1b[34m" + "开始执行" + ansiReset + "......\n" +
		strings.Repeat("=", 50) + "\n"}
}

// CancelTrace 是确认屏上答否时写进工具日志的那一句（老版 logger.Warn 的原话）。
func CancelTrace() contract.Text {
	return contract.Text{S: "执行已取消，SSHFleet工具已退出"}
}

// RefuseEnvelope 是「标签与载荷对不上」那一句的正文。
//
// 正常用不到：它只在纸被造错时才出现（写着命令、装的却是脚本）。真要撞上就是内部缺陷，
// 所以话说清是「这次要做什么与装的东西不一致」，不往使用者身上推。
func RefuseEnvelope(what string) string {
	return HeadCompliance + "：这次要做什么与它的载荷对不上（" + what + "）"
}
