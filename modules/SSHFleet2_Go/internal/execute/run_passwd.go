package execute

// 批量改密：分配 PTY、下发一条空命令，按提示词表认出远端在问什么、喂当前密码与新密码。
//
// 与代填共用底层（PTY 尺寸、写侧钩子、Start + 事件循环、超时与中断契约），但**不带中止词**——
// 「密码过期」那几个词在改密里是**入场信号**，带上中止词会在第 0 秒把自己的会话杀掉
// （方向与代填恰好相反）。
//
// 只写事实：过期信号、早收场、超时中断都进各自的事实格；下发的 `:` 是中间步骤，它的退出码
// 进 Steps——成败与分类由判定产生。

import (
	"context"
	"strings"
	"time"

	"sshfleet2/internal/contract"
	"sshfleet2/internal/message"
)

// passwdCommand 改密会话下发的命令：一条空命令，只为让远端把会话建立流程（含 PAM 的账号
// 检查）走完——密码过期的账号才会把改密对话抛出来，对话发生在命令之前，所以命令是什么
// 并不影响它。
//
// 用 `:`（shell 的内建 no-op）而不是 `exec :`：bash 的 exec 不认内建命令，会报
// `exec: :: 未找到` 并以 127 收场，把改密的结果搅成「执行失败(退出码127)」。
const passwdCommand = ":"

// passwd 一台的改密。
func (c *client) passwd(ctx context.Context, p *contract.Plan, res *contract.NodeResult) {
	session, stdin, err := c.startPTY()
	if err != nil {
		res.ErrorText = c.mergeBanner(err.Error())
		return
	}
	defer func() { _ = session.Close() }()

	out := &lockedBuffer{}
	hook := newPasswdHook(out, p, c.node.Password)
	session.Stdout = hook
	session.Stderr = hook

	send := make(chan string, hook.budget()+1)
	go pump(send, stdin)

	start := time.Now()
	outcome := c.driveSession(ctx, sessionDriver{session: session, events: hook.events, send: send},
		passwdCommand, seconds(p.Run.TimeoutExecute))
	res.Cost = time.Since(start)
	close(send)
	hook.Flush()

	// 采集全文与「最后一次喂入之后」那一段都要脱敏：远端关回显的时机不由我们定，
	// 密码回显可能落在任意一处。
	res.Output = scrubSecrets(trimOuterBlankLines(out.String()), c.node.Password, p.Task.Passwd.NewPassword)
	note := scrubSecrets(hook.replyTail(), c.node.Password, p.Task.Passwd.NewPassword)

	f := endFactsOf(outcome.err, outcome.timedOut, outcome.canceled)
	res.TimedOut = f.timedOut
	res.Cancelled = f.cancelled
	// 下发的 `:` 是中间步骤：它的退出码进 Steps，不顶成结果退出码（改密没有「目的命令」）。
	if f.exitCode != nil {
		res.Steps = append(res.Steps, contract.Step{Name: contract.StepPasswd, ExitCode: f.exitCode})
	}

	// 失败原因：只写**远端自己说的原话**（事实），分类靠判据表按关键词分；远端没说、
	// 或压根轮不到它说（超时 / 中断 / 连接层错误）时不写 Error——那几种形态由上面的事实格
	// 表达，执行侧不编「改密没有完成：…」这类结论。
	if outcome.err != nil {
		if f.exitCode != nil {
			if reason := passwdReason(note); reason != "" {
				res.ErrorText = reason
			}
		} else {
			res.ErrorText = c.mergeBanner(deref(f.errText))
		}
	}

	// 收尾时说清这一摊是怎么结束的（干净收场一个字都不写）。
	clean := isCleanStop(res)
	switch {
	case res.Cancelled:
		res.ErrorText = appendText(res.ErrorText, msg(envAbort, abortCanceled, "interrupted after session established"))
	case outcome.err == nil && f.exitCode == nil:
		// 连接层就失败了：connect.* 报文已在上一步写好，这里没有可补充的事实。
	case clean && !hook.sawEnter():
		// 干净收场、整场没见过过期信号：这台机器本来就不用改。
		res.ErrorText = appendText(res.ErrorText, msg(envPasswd, "not_expired", "no password-expiry prompt seen"))
	case clean:
		// 干净收场且见过信号：一个字都不写。
		//
		// 「一个字都不写」也包括把进场那句服务端提示撤掉——`You are required to change your
		// password immediately` 在改密里是**前提**（它就是入场信号），不是失败原因。留着它，
		// 分类会按「密码过期」认领，干净成功也成了失败；判定那一条要求报错原文为空才算法干净
		// （`ranClean`），两处对不上。
		//
		// 只撤那一句：`ErrorText` 里若还有别的东西（断连、EOF 那类 `:` 拿不到退出码的形态），
		// 照旧留着——整格清空会把真失败抹成成功。
		if res.ErrorText == strings.TrimSpace(c.banner) {
			res.ErrorText = ""
		}
	default:
		res.ErrorText = appendText(res.ErrorText, passwdStuckMessage(passwdShape{
			FedAny:     hook.fedAny(),
			EarlyClose: outcome.aborted,
			TimedOut:   res.TimedOut,
			RemoteSaid: res.ErrorText != "",
		}))
	}
}

// isCleanStop 这次改密是不是「干净收场」：没超时、没中断、没早收场、`:` 也是 0 收场。
func isCleanStop(res *contract.NodeResult) bool {
	if res.TimedOut || res.Cancelled {
		return false
	}
	for _, s := range res.Steps {
		if s.Name == contract.StepPasswd && s.ExitCode != nil && *s.ExitCode != 0 {
			return false
		}
	}
	return true
}

// passwdShape 收尾判定要的几样事实。
type passwdShape struct {
	FedAny     bool
	EarlyClose bool
	TimedOut   bool
	RemoteSaid bool
}

// passwdStuckMessage 没改成的几种形态各说一句（干净收场不调它）。
func passwdStuckMessage(s passwdShape) string {
	switch {
	case s.EarlyClose:
		return msg(envPasswd, "give_up", "no further prompts after answers used up")
	case s.TimedOut && s.FedAny:
		return msg(envPasswd, "no_reply", "password fed but no reply before timeout")
	case s.TimedOut:
		return msg(envPasswd, "no_signal", "no password prompt seen before timeout")
	case s.RemoteSaid:
		// 远端已经说了原因：分类交给判据表按原话认。
		return ""
	case s.FedAny:
		return msg(envPasswd, "give_up", "no further prompts after answers used up")
	default:
		return msg(envPasswd, "no_prompt", "passwd exited without asking for input")
	}
}

// passwdReason 从「最后一次喂入之后」那段采集文本里取原因：从后往前收集非空、非提问行，
// 最多三行，再按原文顺序拼起来。
//
// 两处过滤：以冒号收尾的行是提问（「新的密码：」），不是原因；整行只剩星号的是脱敏占位行，
// 也不是远端说的话。不止取一行：passwd 会把根因与结论分两句说（实测写不进 /etc/shadow 时
// 输出「认证令牌操作错误」＋「密码未更改」），只取最后一句根因就丢了。
func passwdReason(note string) string {
	const maxLines = 3
	lines := strings.Split(note, "\n")
	kept := make([]string, 0, maxLines)
	for i := len(lines) - 1; i >= 0 && len(kept) < maxLines; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" || strings.Trim(line, "*") == "" {
			continue
		}
		if strings.HasSuffix(line, ":") || strings.HasSuffix(line, "：") {
			continue
		}
		kept = append(kept, line)
	}
	for i, j := 0, len(kept)-1; i < j; i, j = i+1, j-1 {
		kept[i], kept[j] = kept[j], kept[i]
	}
	return strings.Join(kept, "\n")
}

// scrubSecrets 把本次喂进去的凭据从采集输出里抹掉。
//
// 远端 PTY 什么时候关回显由 passwd 说了算：我们写进去的字节可能正好落在它关回显之前那一瞬，
// 被终端原样回显出来。这份输出会跟着 Output 进归档、报告与明细表，所以在这里自己擦掉，
// 不指望远端。短密码可能与输出里的无关文字重合——两害相权，宁可看到 `****`。
func scrubSecrets(text string, secrets ...string) string {
	for _, s := range secrets {
		if s == "" {
			continue
		}
		text = strings.ReplaceAll(text, s, "****")
	}
	return text
}

// passwdHook 改密的写侧钩子：采集整备交给 outputTap，匹配交给 passwdMatcher。
type passwdHook struct {
	tap     *outputTap
	matcher *passwdMatcher
	values  map[string]string
	events  chan hookEvent
	fed     int
	fedAt   int
}

func newPasswdHook(out *lockedBuffer, p *contract.Plan, currentPassword string) *passwdHook {
	h := &passwdHook{
		matcher: newPasswdMatcher(p.Tables.PasswdSteps, enterKeywords(p), MatchOptions{
			Regex:         p.Run.MatchRegex,
			CaseSensitive: p.Run.MatchCaseSensitive,
		}),
		values: map[string]string{
			contract.PasswdValueCurrent: currentPassword,
			contract.PasswdValueNew:     p.Task.Passwd.NewPassword,
		},
		events: make(chan hookEvent, budgetOf(p.Tables.PasswdSteps)+1),
	}
	h.tap = newOutputTap(out, h.match)
	return h
}

// budget 全表最多会喂多少次（送信通道的容量按它给，保证钩子永不阻塞）。
func (h *passwdHook) budget() int { return cap(h.events) }

func (h *passwdHook) Write(p []byte) (int, error) { return h.tap.Write(p) }

func (h *passwdHook) Flush() { h.tap.Flush() }

// match 判定命中并发出该喂的值。过期信号在这里只开喂，不终止会话。
func (h *passwdHook) match(chunk string) {
	values, stuck := h.matcher.feed(chunk)
	for _, v := range values {
		h.events <- hookEvent{kind: eventSend, text: h.values[v]}
	}
	if len(values) > 0 {
		// 记下这次喂入时的采集长度：远端对我们这次输入的回应就在它之后。
		h.fedAt = len(h.tap.out.String())
	}
	h.fed += len(values)
	if stuck {
		h.events <- hookEvent{kind: eventAbort}
	}
}

// replyTail 最后一次喂入之后的采集文本——失败原因只可能在这里面。
func (h *passwdHook) replyTail() string {
	var tail string
	h.tap.locked(func() {
		text := h.tap.out.String()
		from := h.fedAt
		if from > len(text) {
			from = len(text)
		}
		tail = text[from:]
	})
	return tail
}

// sawEnter 整场是否出现过入场信号——收尾判定用，与写侧共用同一把锁。
func (h *passwdHook) sawEnter() bool {
	var seen bool
	h.tap.locked(func() { seen = h.matcher.sawEnter })
	return seen
}

// fedAny 是否喂出去过。
func (h *passwdHook) fedAny() bool {
	var any bool
	h.tap.locked(func() { any = h.fed > 0 })
	return any
}

// enterKeywords 入场信号的关键词：判据表里名为「密码过期」的那一类。
//
// 取的是**错误分类表**（`keywords_error.conf` 的第一组），不是改密那张判据表：
// 那句「You are required to change your password immediately」是远端在认证阶段吐的服务端
// 报文，归错误分类认领；代填的中止词也从同一张表、同一个分类名里取（`abortKeywords`）
// ——判据只有一份，改表只改一处。
//
// 那张表为空时，03 已落成「启动即报」的行为例外——这里照旧取空，不另造一句话。
func enterKeywords(p *contract.Plan) []string {
	for _, cat := range p.Tables.Categories {
		if cat.Name == message.CategoryPasswdExpired {
			return cat.Keywords
		}
	}
	return nil
}

// budgetOf 全表最多会喂多少次。
func budgetOf(steps []contract.PasswdStep) int {
	total := 0
	for _, s := range steps {
		total += s.Max
	}
	return total
}

// passwdMatcher 改密的步骤匹配器。
//
// 与代填的游标匹配器共用「在游标之后找最先出现的关键词」这一件，消费语义**相反**：
// 代填一条规则只送一次，改密每条可送 `max` 次——弱新密码被 PAM 拒并重问时，同一个提示
// 要能再答一次。另有一处代填没有的东西：**入场信号**——它出现之前各条提示一律不参战。
type passwdMatcher struct {
	enter    keywordRule
	sawEnter bool
	rules    []passwdRule
	opts     MatchOptions
	tail     string
}

// passwdRule 一条可重复消费的规则。
type passwdRule struct {
	rule  keywordRule
	value string
	left  int
}

func newPasswdMatcher(steps []contract.PasswdStep, enter []string, opts MatchOptions) *passwdMatcher {
	m := &passwdMatcher{enter: newKeywordRule(enter, opts), opts: opts}
	for _, step := range steps {
		m.rules = append(m.rules, passwdRule{
			rule:  newKeywordRule(step.Keywords, opts),
			value: step.Value,
			left:  step.Max,
		})
	}
	return m
}

// passwdHit 一次命中。
type passwdHit struct {
	enter bool
	step  int
	end   int
}

// feed 追加新到的字节，返回本次该喂的值（按出现先后）以及「无话可给」这件事。
func (m *passwdMatcher) feed(chunk string) (values []string, stuck bool) {
	m.tail += chunk
	for {
		h, ok := m.next()
		if !ok {
			break
		}
		m.tail = m.tail[h.end:]
		if h.enter {
			m.sawEnter = true
			continue
		}
		m.rules[h.step].left--
		values = append(values, m.rules[h.step].value)
	}
	// 顺序要紧：先判「又问了一遍」，再清尾部——清了就看不见它了。
	stuck = m.askedAgain()
	if m.exhausted() {
		m.tail = ""
	}
	return values, stuck
}

// askedAgain 游标之后是否出现了已经喂满的那几条提示。
func (m *passwdMatcher) askedAgain() bool {
	for i := range m.rules {
		if m.rules[i].left > 0 {
			continue
		}
		if _, _, _, ok := m.rules[i].rule.find(m.tail, m.opts); ok {
			return true
		}
	}
	return false
}

// next 在游标之后找最先出现的一处命中。
//
// 两态互斥：入场信号还没出现时**只有它参与**；它出现之后轮到还没喂满的步骤。
func (m *passwdMatcher) next() (passwdHit, bool) {
	if !m.sawEnter {
		if _, end, _, ok := m.enter.find(m.tail, m.opts); ok {
			return passwdHit{enter: true, end: end}, true
		}
		return passwdHit{}, false
	}

	best := passwdHit{step: -1}
	bestStart := -1
	for i := range m.rules {
		if m.rules[i].left <= 0 {
			continue
		}
		start, end, _, ok := m.rules[i].rule.find(m.tail, m.opts)
		if !ok || (bestStart >= 0 && start >= bestStart) {
			continue
		}
		best, bestStart = passwdHit{step: i, end: end}, start
	}
	if bestStart < 0 {
		return passwdHit{}, false
	}
	return best, true
}

// exhausted 全表都喂满了。
func (m *passwdMatcher) exhausted() bool {
	for _, r := range m.rules {
		if r.left > 0 {
			return false
		}
	}
	return true
}
