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

// passwdNotExpired 整场没出现过期信号时的收尾文案（未过期机器）。
const passwdNotExpired = "密码未过期，跳过改密"

// passwdGaveUp 早收场但一句原因都没拿到时的收尾文案（远端只反复重问、不作解释）。
const passwdGaveUp = "远端反复重问，工具的答案已经给完了"

// passwdNoPrompt 一句话都没喂出去就超时：远端的提示措辞不在提示词表里。
const passwdNoPrompt = "改密的提示没认出来：提示词表里没有远端那句话的措辞"

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
func (c *Client) RunPasswdChange(ctx context.Context, in PasswdInput, seq int) *Result {
	result := c.newResult(seq)
	defer c.applyBanner(result)

	if !c.connectFor(ctx, result) {
		return result
	}
	defer func() { _ = c.Close() }()

	session, err := c.conn.NewSession()
	if err != nil {
		result.Error = strPtr("创建会话失败 - " + err.Error())
		return result
	}
	defer func() { _ = session.Close() }()

	// tty 模式传 nil：用远端默认（onlcr、echo 都照开）。密码本身不回显（实测 M6），
	// 警告行与提示行照旧进采集范围。
	if err := session.RequestPty(ptyTerm, ptyRows, ptyCols, nil); err != nil {
		result.Error = strPtr("申请终端失败 - " + err.Error())
		return result
	}
	stdin, err := session.StdinPipe()
	if err != nil {
		result.Error = strPtr("创建输入通道失败 - " + err.Error())
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
	result.Output = scrubPasswdSecrets(trimOuterBlankLines(out.String()), c.cfg.Password, in.NewPassword)

	switch {
	case outcome.err != nil:
		if code := extractExitCode(outcome.err); code != nil {
			result.ExitCode = code
		} else {
			result.Error = strPtr(outcome.err.Error())
		}
	default:
		code := 0
		result.ExitCode = &code
	}

	// 失败（早收场或超时）：`error` 写**远端自己说的那句原因**，分类交给判据文件的关键词去分
	//（「密码未被更改」/「BAD PASSWORD」…），工具不另造一个笼统的桶。
	// 拿不到原因行时才回落到工具自己的固定句。
	//
	// 判「有没有中止」必须看 aborted：早收场那条事件不带文本，只看 abortLine 非空的话
	// 会把失败当成正常退出（退出码 0）——实测吃过一次，失败报成了成功。
	if outcome.aborted || outcome.timedOut {
		reason := passwdReason(result.Output)
		switch {
		case reason != "":
		case outcome.timedOut && !hook.fedAny():
			reason = passwdNoPrompt // 一句都没认出来：提示词表里没有那条措辞
		default:
			reason = passwdGaveUp
		}
		result.Error = strPtr(reason)
		result.ExitCode = nil
		return result
	}

	// 收尾判定：整场没出现过入场信号 = 这台机器不需要改密。
	// 只在命令已下发、没被中断、且以退出码 0 正常收场时判——超时、连接失败、命令未能开始时
	// 要保留真实原因，不能被这句话盖掉。
	if !hook.sawEnter() && outcome.begun && !outcome.canceled &&
		result.ExitCode != nil && *result.ExitCode == 0 {
		result.Error = strPtr(passwdNotExpired)
		result.ExitCode = nil
	}
	return result
}

// passwdReason 从采集输出里取「远端最后说的那句原因」：从后往前找第一行既不是空行、
// 也不是提问行的文字。提问一律以冒号收尾（「新的密码：」「重新输入新的密码：」），
// 故按这一条把它排掉；剩下那行就是 passwd 给出的解释（「密码未被更改。」等）。
//
// 取不到就返回空串——调用方自己回落到固定文案。
func passwdReason(output string) string {
	lines := strings.Split(output, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" || strings.HasSuffix(line, ":") || strings.HasSuffix(line, "：") {
			continue
		}
		return line
	}
	return ""
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
	h.fed += len(values)
	if stuck {
		h.events <- hookEvent{kind: eventAbort}
	}
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
