package message

// 确认屏按模式摆全（建站 05 的 09 号单）：四支各自的几行、代填那一串、上传内容树、
// 上传并发建议，以及三道确认门（脚本后缀回退、--sudo 撞 --no-shell、english 撞中文代填）。
//
// 版式逐字照老版 `confirm/confirm.go`：使用者按下回车之前看的就是这一屏，差一个空格都会被
// 当成「工具改版了」。

import (
	"fmt"
	"strconv"
	"strings"

	"sshfleet2/internal/contract"
)

// Bytes 把字节数写成大小：`12.3KB`、`1.0MB`，小于 1KB 写 `512 B`。
//
// 口径照老版 `common.FormatBytes`：1024 进制、一位小数、单位字母大写。
func Bytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGTPE"[exp])
}

// AnswerDescribe 一条代填的呈现形态：内容 → 触发词，参数屏与报告共用。
//
// 内容留空是合法写法（只发一个回车），这里按原义报成「（回车）」，不显示成空。
func AnswerDescribe(a contract.Answer) string {
	value := a.Value
	if value == "" {
		value = "（回车）"
	}
	return value + " → " + strings.Join(a.Triggers, "、")
}

// UploadTree 是上传内容清单：一层树，标题 + 根 + 根下那一层。
//
// 条目从纸上来（`Task.Upload.TopEntries`），不由文件表反推——反推会漏掉空子目录，
// 而被过滤的链接也要列出来（不让它静默消失）。
func UploadTree(p *contract.Plan) Line {
	task := p.Task.Upload
	if task == nil {
		return Line{}
	}
	var b strings.Builder
	b.WriteString("\n" + ansiYellow + "📁 上传文件/目录内容 (-u 选项):" + ansiReset + "\n")

	name := baseName(task.Source)
	if !task.RootIsDir {
		b.WriteString("└── " + name + " (文件)\n")
		return Line{s: b.String()}
	}
	b.WriteString("└── " + name + "/\n")
	for i, item := range task.TopEntries {
		prefix := "    ├──"
		if i == len(task.TopEntries)-1 {
			prefix = "    └──"
		}
		if item.IsDir {
			b.WriteString(prefix + " " + item.Name + "/\n")
			continue
		}
		b.WriteString(prefix + " " + item.Name + "\n")
	}
	return Line{s: b.String()}
}

// UploadAdvice 是并发建议那一句：总大小、按阈值建议几个、当前几个。
//
// 前面留一个空行：它紧跟在上传内容树之后，不留空会跟那棵树读成一块。
func UploadAdvice(total int64, advice, current int) Line {
	return Line{s: fmt.Sprintf("\n%s上传总大小 %s，按配置里的阈值建议同时传 %d 个（当前是 %d 个）。%s\n",
		ansiYellow, Bytes(total), advice, current, ansiReset)}
}

// UploadAdvicePrompt 是那条建议的问句。
func UploadAdvicePrompt(advice int) Line {
	return Line{s: fmt.Sprintf("改成 %d 个？", advice)}
}

// UploadAdviceTrace 是非交互（--yes）自动采纳建议时的留痕。
func UploadAdviceTrace(advice, current int) Line {
	return Line{s: fmt.Sprintf("非交互模式（--yes）自动采纳上传并发建议：%d 个（当前 %d 个）", advice, current)}
}

// FallbackPrompt 是脚本后缀没配到解释器时那一问。
func FallbackPrompt(ext, interpreter string) Line {
	return Line{s: fmt.Sprintf(
		"脚本后缀 %q 没有配置解释器\n"+
			"  想换别的解释器：在 %s 的 [interpreter.script] 里加一行，如 %q = \"解释器名\"\n"+
			"  继续用 %s 作为解释器吗？",
		ext, ConfigShownPath, ext, interpreter)}
}

// FallbackTrace 是非交互（--yes）下跳过这一问时的留痕。
func FallbackTrace(ext, interpreter string) Line {
	return Line{s: fmt.Sprintf("非交互模式（--yes）跳过确认：脚本后缀 %q 未配解释器，直接回退到 %s", ext, interpreter)}
}

// FallbackDeclined 是答否之后那一行，随后当场退出。
func FallbackDeclined(ext string) Line {
	return Line{s: fmt.Sprintf("%s已取消：脚本后缀 %q 没有配置解释器，本次执行终止%s\n", ansiYellow, ext, ansiReset)}
}

// NoShellDropsSudoNotice 是 `--sudo` 与 `--no-shell` 同给时的说明：先说清 sudo 不生效，
// 再问一句。
//
// 这一道门老版有，建站 04 的 A/B 没测到它；本阶段既然要动确认屏，顺手补齐
// （04 漏下的那道门，记在 09 号单里）。
func NoShellDropsSudoNotice() Line {
	return Line{s: ansiYellow + "⚠ --sudo 与 --no-shell 同时给出" + ansiReset + "\n" +
		"  --no-shell 是原样下发、工具不附加任何东西，所以 --sudo 不生效：\n" +
		"  本次将以【登录用户】身份执行，命令里不会带 sudo。\n" +
		"  要 root 身份，请去掉 --no-shell；要原样跑，请去掉 --sudo。\n"}
}

// NoShellDropsSudoPrompt 是那道门的问句。
func NoShellDropsSudoPrompt() Line {
	return Line{s: "\n" + ansiYellow + "继续执行？" + ansiReset}
}

// NoShellDropsSudoTrace 是非交互（--yes）下跳过这一问时的留痕。
func NoShellDropsSudoTrace() Line {
	return Line{s: "--sudo 被 --no-shell 忽略：本次以登录用户身份执行，命令里不含 sudo"}
}

// ChineseAnswerWarning 是「english 开着、代填值里却有汉字」那段说明。
//
// 这一条是**跑不跑**的差别，不是措辞差别：english = true 时目标机提示语一律英文，
// 代填值按英文准备才对，中文值一定失配。触发词不受此限——服务端报文（密码过期等）
// 不受 english 影响，仍可能是中文。
func ChineseAnswerWarning(p *contract.Plan, indexes []int) Line {
	var b strings.Builder
	b.WriteString(ansiYellow + "⚠ [locale] english = true，但代填值里有中文" + ansiReset + "\n")
	b.WriteString("  english = true 时目标机的提示语一律英文，代填值（第 1 列）按英文准备才对：\n")
	answers := planAnswers(p)
	for _, i := range indexes {
		if i < 0 || i >= len(answers) {
			continue
		}
		b.WriteString("    第 " + strconv.Itoa(i+1) + " 条代填值是「" + answers[i].Value + "」\n")
	}
	b.WriteString("  触发词不受此限——服务端报文（如密码过期）不受 english 影响，仍可能是中文。\n")
	b.WriteString("  要跑中文代填值，请把配置里 [locale] 的 english 改成 false。\n")
	return Line{s: b.String()}
}

// ChineseAnswerPrompt 是那道门的问句（缺省否：回车即退出）。
func ChineseAnswerPrompt() Line {
	return Line{s: "\n" + ansiYellow + "仍要继续执行？" + ansiReset}
}

// ChineseAnswerCancelled 是非交互（--yes）下直接取消时那一行。
func ChineseAnswerCancelled() Line {
	return Line{s: ansiYellow + "⚠ [locale] english = true 但代填值含中文：已取消执行（回命令行改代填值，或把 english 改成 false）" + ansiReset}
}

// ChineseAnswerTrace 是取消时写进工具日志的那一句。
func ChineseAnswerTrace() Line {
	return Line{s: "执行已取消（english = true 与中文代填值同时给出）"}
}

// planAnswers 这张纸上配的代填表（命令与脚本载荷各一份，其余模式没有）。
func planAnswers(p *contract.Plan) []contract.Answer {
	switch {
	case p.Task.Command != nil:
		return p.Task.Command.Answers
	case p.Task.Script != nil:
		return p.Task.Script.Answers
	}
	return nil
}

// baseName 取路径最后一段：三种路径规则混着来（本机、远端、Windows 反斜杠），
// 所以两种分隔符都按分隔符看——它只用来显示，不参与任何拼装。
func baseName(p string) string {
	p = strings.TrimRight(p, `/\`)
	if i := strings.LastIndexAny(p, `/\`); i >= 0 {
		return p[i+1:]
	}
	return p
}
