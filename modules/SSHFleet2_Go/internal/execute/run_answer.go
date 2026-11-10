package execute

// 代填：给了 `-a` 时改走交互分支——分配 PTY、发纸上那条下发行、会话 stdin 整条让给代填。
//
// 与普通路径的差别只有三处：分配 PTY、正文改经命令行承载（解析站已经 base64 编好）、
// 会话 stdin 整条让给代填；其余（结果字段、超时与中断语义）同源。
//
// 命中触发词就填、命中中止词就停；没送出去的条目在收尾时说清（序号进纸，话由
// message 按事实造）。

import (
	"context"
	"strconv"
	"time"

	"sshfleet2/internal/contract"
	"sshfleet2/internal/message"
)

// answersOf 这次配的代填表（命令与脚本载荷各一份）。
func answersOf(p *contract.Plan) []contract.Answer {
	switch {
	case p.Task.Command != nil:
		return p.Task.Command.Answers
	case p.Task.Script != nil:
		return p.Task.Script.Answers
	}
	return nil
}

// runAnswer 走交互分支跑这次的任务。
func (c *client) runAnswer(ctx context.Context, p *contract.Plan, res *contract.NodeResult) {
	answers := answersOf(p)
	session, stdin, err := c.startPTY()
	if err != nil {
		res.ErrorText = c.mergeBanner(err.Error())
		return
	}
	defer func() { _ = session.Close() }()

	out := &lockedBuffer{}
	hook := newAnswerHook(out, answers, MatchOptions{Regex: p.Run.MatchRegex, CaseSensitive: p.Run.MatchCaseSensitive}, c.abortKeywords(p))
	session.Stdout = hook
	session.Stderr = hook

	send := make(chan string, len(answers)+1)
	go pump(send, stdin)

	start := time.Now()
	outcome := c.driveSession(ctx, sessionDriver{session: session, events: hook.events, send: send},
		p.Run.Downlink, seconds(p.Run.TimeoutExecute))
	res.Cost = time.Since(start)
	close(send)
	hook.Flush()

	res.Output = trimOuterBlankLines(out.String())

	f := endFactsOf(outcome.err, outcome.timedOut, outcome.canceled)
	res.ExitCode = f.exitCode
	res.TimedOut = f.timedOut
	res.Cancelled = f.cancelled

	// 代填的收尾三态。中止词优先（命中过的场次不判未命中，否则即时归因会被盖掉）；
	// 中断有自己的去处（任务已取消），也不判未命中。
	missed := hook.pending()
	res.Answer.Missed = missed
	switch {
	case outcome.abortLine != "":
		res.Answer.AbortLine = outcome.abortLine
	case outcome.begun && !outcome.canceled && len(missed) == 0 &&
		outcome.timedOut && len(answers) > 0:
		// 表已送空仍超时：工具分不出「脚本卡在没配触发词的提示上」与「任务本身耗时长」，
		// 但「表已空」是确定的事实。
		res.Answer.Exhausted = true
	}

	// 收尾时说清这一摊是怎么结束的：代填没送完 / 表空了还在等 / 被中断。
	// 服务端提示排在报文之后（老版次序：原文 → 报文 → 提示 → 未送出的代填），所以提示
	// 留到这儿再接：先接报文、再接提示，别让报文落到提示后面。
	reason := answerMessage(len(missed), res.Answer.Exhausted, res.TimedOut)
	if outcome.canceled {
		reason = abortMessage(true)
	}
	res.ErrorText = c.mergeBanner(appendText(deref(f.errText), reason))
	// 末尾这一段逐条列出没送出去的：被按停不等于那些条已经送出去了，中断时照列（老版同）。
	res.ErrorText = appendText(res.ErrorText, message.AnswerMissedNote(answers, missed))
}

// answerMessage 代填收尾那句报文。
//
// 优先说「几条没送出去」（最具体、可操作）；其次说「全送出去了仍在等」；其余形态不说话。
func answerMessage(missed int, exhausted, timedOut bool) string {
	if missed > 0 {
		unit := "answers"
		if missed == 1 {
			unit = "answer"
		}
		return msg(envAnswer, "missed", strconv.Itoa(missed)+" "+unit+" not delivered")
	}
	if exhausted && timedOut {
		return msg(envAnswer, "exhausted_timeout", "all answers sent but no output before timeout")
	}
	return ""
}

// abortKeywords 中止词：取判据表里「密码过期」那一类（分类名是常量，随纸来）。
//
// 代填碰上「密码过期」这种提示就地收手——继续填下去只是把密码往一个不肯收的地方灌。
func (c *client) abortKeywords(p *contract.Plan) []string {
	for _, cat := range p.Tables.Categories {
		if cat.Name == message.CategoryPasswdExpired {
			return cat.Keywords
		}
	}
	return nil
}

// answerHook 代填的写侧钩子：采集整备交给 outputTap，匹配交给 cursorMatcher。
type answerHook struct {
	tap     *outputTap
	values  []string
	answers *cursorMatcher
	abort   *cursorMatcher
	events  chan hookEvent
}

func newAnswerHook(out *lockedBuffer, answers []contract.Answer, opts MatchOptions, abortKeywords []string) *answerHook {
	rules := make([][]string, 0, len(answers))
	values := make([]string, 0, len(answers))
	for _, a := range answers {
		rules = append(rules, a.Triggers)
		values = append(values, a.Value)
	}
	h := &answerHook{
		values:  values,
		answers: newCursorMatcher(rules, opts),
		events:  make(chan hookEvent, len(answers)+2), // 容量够放下全部命中，钩子永不阻塞
	}
	if len(abortKeywords) > 0 {
		h.abort = newCursorMatcher([][]string{abortKeywords}, opts)
	}
	h.tap = newOutputTap(out, h.match)
	return h
}

func (h *answerHook) Write(p []byte) (int, error) { return h.tap.Write(p) }

func (h *answerHook) Flush() { h.tap.Flush() }

// pending 还没送出的代填条目序号（1 起）。
func (h *answerHook) pending() []int {
	var out []int
	h.tap.locked(func() { out = h.answers.pending() })
	return out
}

// match 判定命中并发信号。中止词优先：命中即终止，同块里的代填不再送出。
func (h *answerHook) match(chunk string) {
	if h.abort != nil {
		if hits := h.abort.feed(chunk); len(hits) > 0 {
			h.events <- hookEvent{kind: eventAbort, text: hits[0].line}
			return
		}
	}
	for _, h1 := range h.answers.feed(chunk) {
		h.events <- hookEvent{kind: eventSend, text: h.values[h1.rule]}
	}
}
