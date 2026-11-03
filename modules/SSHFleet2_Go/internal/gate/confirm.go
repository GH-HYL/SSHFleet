package gate

// 确认那一段（建站 05 的 09 号单）：三道门 + 确认屏 + 上传并发建议。
//
// 次序照老版 `main.go` 第 7 步：脚本后缀回退那一问在确认屏之前（`cli.ConfirmInterpreterFallback`），
// 然后是 confirm.Confirm 里的两道门（--sudo 撞 --no-shell、english 撞中文代填），再出屏。
//
// 闸门在这一段仍然只判不放：屏与问句由 message 造、打屏与收行归 ask。这里只决定
// 「答了否算不算停」「采纳了建议之后纸上那一格取什么值」。

import (
	"unicode"

	"sshfleet2/internal/ask"
	"sshfleet2/internal/contract"
	"sshfleet2/internal/mask"
	"sshfleet2/internal/message"
)

// confirmFlow 是确认那一段的全流程。
func confirmFlow(p *contract.Plan) (Refusal, message.Line) {
	// 一、脚本后缀没配到解释器：当场问一句是否用回退解释器继续。
	if r, note := fallbackConfirm(p); r != nil || note.Text() != "" {
		return r, note
	}
	// 二、`--sudo` 与 `--no-shell` 同给：sudo 不生效，先说清再问一句。
	if r, note := noShellSudoGate(p); r != nil || note.Text() != "" {
		return r, note
	}
	// 三、english 开着而代填值里有汉字：一定失配，提醒后问一句（缺省否）。
	if r, note := chineseAnswerGate(p); r != nil || note.Text() != "" {
		return r, note
	}

	// 四、非交互（--yes）：不出参数屏、不提问，但上传并发建议照旧生效。
	if p.Run.Yes {
		if note := applyConcurrencyAdvice(p); note.Text() != "" {
			ask.Ask(message.ConfirmSkippedLine(), true, true)
			return nil, note
		}
		ask.Ask(message.ConfirmSkippedLine(), true, true)
		return nil, message.ConfirmSkippedTrace()
	}

	// 五、参数屏 → 上传内容树 → 并发建议（可能重显一次屏）→ 确认问句。
	ask.Show(message.ConfirmScreen(p, mask.InlineRecordIf(p.Resources.Manifest), maskedNewPassword(p), false))
	if p.Task.Upload != nil {
		ask.Show(message.UploadTree(p))
	}
	adopted, r := askUploadConcurrency(p)
	if r != nil {
		return r, message.Line{}
	}
	if adopted.Text() != "" {
		// 采纳了建议：参数屏重显一次，并发数那一行加粗并注明（ADR-0040 决定三）。
		ask.Show(message.ConfirmScreen(p, mask.InlineRecordIf(p.Resources.Manifest), maskedNewPassword(p), true))
	}

	switch ask.AskWith(message.Line{}, message.ExecConfirmPrompt(), true) {
	case ask.NoInput:
		// 屏幕上「输入结束，操作已取消」已经由 ask 打过，这里不再叠一句。
		return newRefusal(message.Line{}), message.Line{}
	case ask.No:
		return newRefusal(message.CancelLine()), message.CancelTrace()
	}
	// 答是之后补一句「这次少传了什么」：--yes 那条路老版不印（它在跳过确认处就返回了），
	// 所以这一句只挂在这里。执行期日志里另有一份（在开头两行之前）。
	if p.Task.Upload != nil && len(p.Task.Upload.Skipped) > 0 {
		ask.Show(message.UploadSkippedLine(p.Task.Upload.Skipped))
	}
	return nil, adopted
}

// maskedNewPassword 改密屏上那个新密码的形态：由闸门遮一次递过去（`message` 只许依赖契约）。
func maskedNewPassword(p *contract.Plan) string {
	if p.Task.Passwd == nil {
		return ""
	}
	return mask.Secret(p.Task.Passwd.NewPassword)
}

// fallbackConfirm 脚本后缀没配到解释器时那一问。
//
// 非交互（--yes）下按确认放行并留痕（老版就是这个缺省）。答否 ⇒ 拒绝、当场退出、
// **不留归档目录**——所以它是拒绝，不是起不来。
func fallbackConfirm(p *contract.Plan) (Refusal, message.Line) {
	if p.Task.Script == nil || p.Run.FallbackExt == "" {
		return nil, message.Line{}
	}
	if p.Run.Yes {
		// 非交互下整屏不出：老版在 --yes 里跳过了这一问，屏上只剩「跳过确认」那一行，
		// 回退到 bash 这件事只留痕。
		ask.Ask(message.ConfirmSkippedLine(), true, true)
		return nil, message.FallbackTrace(p.Run.FallbackExt, p.Run.Interpreter)
	}
	switch ask.AskQuestion(message.FallbackHint(p.Run.FallbackExt), message.FallbackQuestion(p.Run.Interpreter), false) {
	case ask.NoInput:
		return newRefusal(message.Line{}), message.Line{}
	case ask.No:
		ask.Show(message.FallbackDeclined(p.Run.FallbackExt))
		return newRefusal(message.Line{}), message.CancelTrace()
	}
	return nil, message.Line{}
}

// noShellSudoGate `--sudo` 与 `--no-shell` 同给时那道门（04 漏下的一道，本阶段补齐）。
//
// 老版在确认屏之前做这件事，这里同一位置。答否当场退出；`--yes` 下上屏一行并留痕。
func noShellSudoGate(p *contract.Plan) (Refusal, message.Line) {
	if !p.Run.NoShell || !p.Run.Sudo {
		return nil, message.Line{}
	}
	if p.Run.Yes {
		ask.Ask(message.NoShellDropsSudoNotice(), true, true)
		return nil, message.NoShellDropsSudoTrace()
	}
	switch ask.AskQuestion(message.NoShellDropsSudoNotice(), message.NoShellDropsSudoPrompt(), true) {
	case ask.NoInput:
		return newRefusal(message.Line{}), message.Line{}
	case ask.No:
		return newRefusal(message.CancelLine()), message.CancelTrace()
	}
	return nil, message.Line{}
}

// chineseAnswerGate english 开着而代填值里有汉字的那道门。
//
// 触发词不查：服务端报文（密码过期、sshd 提示）不受 english 影响，仍可能是中文。
// 全角字母数字与全角标点不算汉字——拦它们就过了。
//
// 非交互（--yes）没有「他」可提醒，而这是一定失配的逻辑错误，放行就是整批白跑：
// 直接取消并留痕，不给「跳过确认就继续」的口子。
func chineseAnswerGate(p *contract.Plan) (Refusal, message.Line) {
	if !p.Run.English {
		return nil, message.Line{}
	}
	var hit []int
	for i, entry := range answersOf(p) {
		if hasHan(entry.Value) {
			hit = append(hit, i)
		}
	}
	if len(hit) == 0 {
		return nil, message.Line{}
	}
	if p.Run.Yes {
		ask.Show(message.ChineseAnswerCancelled())
		return newRefusal(message.Line{}), message.ChineseAnswerTrace()
	}
	ask.Show(message.ChineseAnswerWarning(p, hit))
	switch ask.AskQuestion(message.Line{}, message.ChineseAnswerPrompt(), false) {
	case ask.NoInput:
		return newRefusal(message.Line{}), message.Line{}
	case ask.No:
		return newRefusal(message.CancelLine()), message.ChineseAnswerTrace()
	}
	return nil, message.Line{}
}

// askUploadConcurrency 上传并发建议那一问。
//
// 三条判据：算得出建议（0 表示没有）、建议值不等于当前生效值、这次确实是上传。
// 答是 ⇒ 改纸上那一格（`RunParams.Concurrency`，闸门唯一准改的格子，ADR-0040），
// 并把参数屏重显一次；答否 ⇒ 不动。
//
// 非交互（--yes）按采纳处理并留痕——老版那条路也走同一处。
func askUploadConcurrency(p *contract.Plan) (message.Line, Refusal) {
	if p.Task.Upload == nil {
		return message.Line{}, nil
	}
	advice := p.Run.ConcurrencyAdvice
	if advice == 0 {
		return message.Line{}, nil
	}
	current := concurrencyInEffect(p)
	if advice == current {
		return message.Line{}, nil
	}

	if p.Run.Yes {
		// 非交互下不问，但那一句「建议同时传 N 个」照印：老版 --yes 也把它打在屏上，
		// 让人知道这次不是按原值跑的。
		ask.Show(message.UploadAdvice(p.Task.Upload.TotalBytes, advice, current))
		p.Run.Concurrency = advice
		return message.UploadAdviceTrace(advice, current), nil
	}

	ask.Show(message.UploadAdvice(p.Task.Upload.TotalBytes, advice, current))
	switch ask.AskQuestion(message.Line{}, message.UploadAdvicePrompt(advice), true) {
	case ask.NoInput:
		return message.Line{}, newRefusal(message.Line{})
	case ask.No:
		return message.Line{}, nil // 保留原值继续执行
	}
	p.Run.Concurrency = advice
	// 这一行既是「屏要重显」的信号，也是 ADR-0040 要的那条留痕（写进工具日志）。
	return message.UploadAdviceAdopted(advice, current), nil
}

// applyConcurrencyAdvice 是非交互那条路的同一条判据（不出屏、不问，直接采纳）。
func applyConcurrencyAdvice(p *contract.Plan) message.Line {
	note, _ := askUploadConcurrency(p)
	return note
}

// concurrencyInEffect 当前生效的并发数：没给 -n（0）时就是节点数。
func concurrencyInEffect(p *contract.Plan) int {
	if p.Run.Concurrency != 0 {
		return p.Run.Concurrency
	}
	return len(p.Nodes)
}

// answersOf 这次配的代填表（命令与脚本载荷各一份，其余模式没有）。
func answersOf(p *contract.Plan) []contract.Answer {
	switch {
	case p.Task.Command != nil:
		return p.Task.Command.Answers
	case p.Task.Script != nil:
		return p.Task.Script.Answers
	}
	return nil
}

// hasHan 字符串里有没有汉字（含扩展 A 区）。全角字母数字与标点不算。
func hasHan(s string) bool {
	for _, r := range s {
		if unicode.Is(unicode.Han, r) {
			return true
		}
	}
	return false
}
