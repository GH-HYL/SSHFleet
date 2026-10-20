package ssh

import (
	"context"
	"io"
	"strings"
	"time"
)

// passwdCommand 改密会话下发的命令：一条空命令，只为让远端把会话建立流程（含 PAM 的账号
// 检查）走完——密码过期的账号才会把改密对话抛出来，对话发生在命令之前，所以命令是什么
// 并不影响它。
//
// 用 `:`（shell 的内建 no-op）而不是 `exec :`：bash 的 exec 不认内建命令，会报
// `exec: :: 未找到` 并以 127 收场，把改密的结果搅成「执行失败(退出码127)」（2026-09-29 实测）。
// 命令跑完 shell 自己退出，也不会留一个还在等输入的登录 shell。
const passwdCommand = ":"

// passwdCommand 下发的那条命令见上。
//
// 这里不再有「改密没有完成：…」那几句兜底文案（2026-10-14 删）：它们是执行侧自己编的
// **结论**，而执行侧只该记事实（结果统一规格 D3）。四句各自对应的形态改成结构字段
//（PasswdFedAny / TimedOut / PasswdEarlyClose），由判定层直接定性——分类不再依赖那句
// 文案，改措辞也就不会让分类静默失效。

// PasswdInput 改密会话的入场值。
//
// **当前密码不在这里**：它就是清单里那台机器的登录密码，客户端配置里已经有（见 NewClient）。
// 新密码是全场一个值，与代填一样由所有节点共用同一份入场值。
type PasswdInput struct {
	NewPassword   string
	Prompts       *PasswdPrompts
	EnterKeywords []string // 入场信号：取判据文件「密码过期」分类的关键词
	Match         MatchOptions
}

// RunPasswdChange 对一台机器执行改密：分配 PTY、下发空命令、按提示词表喂入，
// 直到会话自己结束或 `-t` 到点。
//
// 与代填交互分支复用同一套底层（PTY 尺寸、写侧钩子、Start + 事件循环、超时与中断契约），
// 但**不带中止词**——「密码过期」那几个词在改密里是入场信号，带上中止词会在第 0 秒把
// 自己的会话杀掉（方向与代填恰好相反）。
//
// 只写事实字段，不写结论：过期信号、早收场、超时中断都进各自的事实格；下发的 `:` 是
// 中间步骤（D33），它的退出码进 Steps——成败与分类（含「密码未过期」）由结果判定产生。
func (c *Client) RunPasswdChange(ctx context.Context, in PasswdInput, seq int) *Result {
	result := c.newResult(seq)
	defer c.captureBanner(result) // 服务端提示无条件写独立字段（D34）；是否参与匹配由判定按模式决定

	if !c.connectFor(ctx, result) {
		return result
	}
	defer func() { _ = c.Close() }()

	session, err := c.conn.NewSession()
	if err != nil {
		result.Error = strPtr(Message(EnvSession, sessionCreateFailed, err.Error()))
		return result
	}
	defer func() { _ = session.Close() }()

	// tty 模式传 nil：用远端默认（onlcr、echo 都照开）。密码本身不回显（实测 M6），
	// 警告行与提示行照旧进采集范围。
	if err := session.RequestPty(ptyTerm, ptyRows, ptyCols, nil); err != nil {
		result.Error = strPtr(Message(EnvSession, sessionPtyFailed, err.Error()))
		return result
	}
	stdin, err := session.StdinPipe()
	if err != nil {
		result.Error = strPtr(Message(EnvSession, sessionChannelFailed, err.Error()))
		return result
	}

	out := &lockedBuffer{}
	hook := newPasswdHook(out, in, c.cfg.Password)
	session.Stdout = hook
	session.Stderr = hook

	// 送内容放在自己的协程里：写输入通道在窗口满时会阻塞，不能因此卡住消费循环
	// （消费一停，x/crypto 的复制协程就停，远端输出回压），也不能挡住 ctx 到点。
	send := make(chan string, in.Prompts.stepBudget()+1)
	go func() {
		for text := range send {
			_, _ = io.WriteString(stdin, text+answerEOL)
		}
	}()

	execStart := time.Now()
	outcome := c.driveSession(ctx, sessionDriver{session: session, events: hook.events, send: send}, passwdCommand)
	result.ExecCostTime = time.Since(execStart).Seconds()
	close(send)
	hook.Flush()
	// 采集全文进结果；失败原因另取「最后一次喂入之后」那一段——两段都要脱敏，
	// 远端关回显的时机不由我们定，密码回显可能落在任意一处。
	note := scrubPasswdSecrets(hook.replyTail(), c.cfg.Password, in.NewPassword)
	result.Output = scrubPasswdSecrets(trimOuterBlankLines(out.String()), c.cfg.Password, in.NewPassword)

	// 事实：会话起没起、怎么收的场、改密对话走到哪一步。下发的 `:` 是中间步骤，
	// 它的退出码进 Steps（D33）——CommandExitCode 恒为 nil，改密没有"目的命令"。
	result.SessionBegun = outcome.begun
	f := endFactsOf(outcome.err, outcome.timedOut, outcome.canceled)
	result.TimedOut = f.timedOut
	result.Canceled = f.canceled
	result.PasswdEarlyClose = outcome.aborted
	result.EnterSignalSeen = hook.sawEnter()
	// 有没有喂进去过——「工具没认出远端在问什么」与「喂了但远端没反应」由此分开，
	// 判定层据此给不同分类。读到此刻为止的计数（会话已结束、写侧也已收场）。
	result.PasswdFedAny = hook.fedAny()
	// passwd 自己以非 0 收场：那个码只说明「没改成」，原因在它的输出里——
	// 归 Steps 留痕，不顶成结果退出码（否则判定会给出「执行失败(退出码N)」，
	// 把远端说的原因整个盖掉，2026-09-29 修）。
	if f.exitCode != nil {
		result.Steps = append(result.Steps, StepResult{Name: StepPasswd, ExitCode: f.exitCode})
	}

	// 失败原因：只写**远端自己说的原话**（事实），分类靠判据表按关键词分；远端没说、
	// 或压根轮不到它说（超时 / 中断 / 连接层错误）时不写 Error——那种形态由上面那几个
	// 结构字段表达，判定层直接定性，执行侧不编「改密没有完成：…」这类结论（D3）。
	// 「超时 / 中断先判、否则盖掉远端原因」的次序警示写在 endFactsOf 的注释里（单点），此处不重复。
	//
	// 注意成功路径（outcome.err == nil 且未超时未早收场）整段不进来：那时 note 里是
	// 「passwd：已成功更新密码」这类**成功**回执，当成失败原因写进去会让判定把它当失败
	//（2026-10-14 实测踩过）。
	if outcome.err != nil {
		if f.exitCode != nil {
			// passwd 自己以非 0 收场：有原话就照实写，没有就留给下面的报文。
			if reason := passwdReason(note); reason != "" {
				result.Error = strPtr(reason)
			}
		} else {
			// 连接层 / 会话层的错误（连不上、会话建不起来、命令没能开始）：没有可读的
			// 远端输出，保留原文——这是事实（SSH 库自己的报错），也是分类依据。
			result.Error = f.errText
		}
	}

	// 收尾时说清这一摊是怎么结束的（干净收场一个字都不写）：
	// 被中断 / 卡在哪一步 / 这台机器本来就不用改。一律**追加**，不盖远端原话。
	switch {
	case result.Canceled:
		result.Error = appendAbort(result.Error, true)
	case outcome.err == nil && f.exitCode == nil:
		// 连接层就失败了：connect.* 报文已在 connectFor 里写好，这里没有可补充的事实。
	case isCleanStop(result):
		result.Error = appendMessage(result.Error, PasswdNotExpiredMessage(result.EnterSignalSeen))
	default:
		result.Error = appendMessage(result.Error, PasswdStuckMessage(PasswdShape{
			FedAny:          result.PasswdFedAny,
			EarlyClose:      result.PasswdEarlyClose,
			TimedOut:        result.TimedOut,
			EnterSignalSeen: result.EnterSignalSeen,
			RemoteSaid:      deref(result.Error) != "",
		}))
	}
	return result
}

// isCleanStop 这次改密是不是「干净收场」：没超时、没中断、没早收场、passwd 自己也是 0 收场。
// 与判定层 succeeded() 里那条同口径；执行侧用它决定要不要发「密码未过期」那句。
func isCleanStop(res *Result) bool {
	if res.TimedOut || res.Canceled || res.PasswdEarlyClose {
		return false
	}
	for _, s := range res.Steps {
		if s.Name == StepPasswd && s.ExitCode != nil && *s.ExitCode != 0 {
			return false
		}
	}
	return true
}

// passwdReasonLines error 里最多带几行远端原话。
const passwdReasonLines = 3

// passwdReason 从「最后一次喂入之后」那段采集文本里取原因：从后往前收集非空、非提问行，
// 最多 passwdReasonLines 行，再按原文顺序拼起来。
//
// 两处过滤：
//   - 以冒号收尾的行是提问（「新的密码：」「重新输入新的密码：」），不是原因；
//   - 整行只剩星号的是脱敏占位行（我们喂进去的密码被远端回显，见 scrubPasswdSecrets），
//     也不是远端说的话——实测它会把「远端压根没回应」误当成"有原因"（2026-09-29）。
//
// 为什么不止取一行：passwd 会把根因与结论分两句说——实测写不进 /etc/shadow 时输出的是
// 「passwd：认证令牌操作错误」＋「passwd：密码未更改」。只取最后一句的话根因就丢了。
//
// 取不到就返回空串——调用方自己回落到固定文案。
func passwdReason(note string) string {
	lines := strings.Split(note, "\n")
	kept := make([]string, 0, passwdReasonLines)
	for i := len(lines) - 1; i >= 0 && len(kept) < passwdReasonLines; i-- {
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
		kept[i], kept[j] = kept[j], kept[i] // 翻回原文顺序
	}
	return strings.Join(kept, "\n")
}

// scrubPasswdSecrets 把本次喂进去的凭据从采集输出里抹掉。
//
// 远端 PTY 什么时候关回显由 passwd 说了算：我们写进去的字节可能正好落在它关回显之前
// 那一瞬，被终端原样回显出来（2026-09-29 实测到一次——新密码整行出现在归档的
// terminal-output.txt 里）。这份输出会跟着 Output 进归档、报告与明细表，所以在这里
// 自己擦掉，不指望远端。
//
// 短密码可能与输出里的无关文字重合，抹掉会损失一点诊断信息——两害相权，宁可看到
// `****`，也不能把凭据写进归档。
func scrubPasswdSecrets(text string, secrets ...string) string {
	for _, s := range secrets {
		if s == "" {
			continue
		}
		text = strings.ReplaceAll(text, s, "****")
	}
	return text
}

// passwdHook 改密的写侧钩子：采集整备交给 outputTap，匹配交给 passwdMatcher。
// 与代填的钩子同一形态，差别在匹配语义与中止条件——改密只有一种中止：
// 全表喂满而远端还在问（无话可给）。
type passwdHook struct {
	tap     *outputTap
	matcher *passwdMatcher
	values  map[PasswdValue]string
	events  chan hookEvent
	fed     int // 已喂出去几次：收尾文案要分得清「提示没认出来」与「喂了但它不收」
	fedAt   int // 最后一次喂入时的采集长度：失败原因只可能在它之后（见 replyTail）
}

func newPasswdHook(out *lockedBuffer, in PasswdInput, currentPassword string) *passwdHook {
	h := &passwdHook{
		matcher: newPasswdMatcher(in.Prompts, in.EnterKeywords, in.Match),
		values: map[PasswdValue]string{
			PasswdValueCurrent: currentPassword,
			PasswdValueNew:     in.NewPassword,
		},
		// 容量按全表预算给足：命中即发、外加一条中止事件，钩子永不阻塞
		events: make(chan hookEvent, in.Prompts.stepBudget()+1),
	}
	h.tap = newOutputTap(out, h.match)
	return h
}

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
		// 不记的话只能在整段输出里找原因，而那样会捡到警告行与对话开场白（实测吃过）。
		h.fedAt = len(h.tap.out.String())
	}
	h.fed += len(values)
	if stuck {
		h.events <- hookEvent{kind: eventAbort}
	}
}

// replyTail 最后一次喂入之后的采集文本——失败原因只可能在这里面。
// 一次都没喂过时返回空串（调用方按「提示没认出来」处理），而不是退回全文。
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

// sawEnter 整场是否出现过入场信号——收尾判定用，与钩子的写侧共用同一把锁。
func (h *passwdHook) sawEnter() bool {
	var seen bool
	h.tap.locked(func() { seen = h.matcher.sawEnterSignal() })
	return seen
}

// fedAny 是否喂出去过——收尾文案用，同一把锁。
func (h *passwdHook) fedAny() bool {
	var any bool
	h.tap.locked(func() { any = h.fed > 0 })
	return any
}
