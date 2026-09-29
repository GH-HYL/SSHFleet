package ssh

import (
	"context"
	"io"
	"time"
)

// passwdCommand 改密会话下发的命令：一条空命令，只为让远端把会话建立流程（含 PAM 的账号
// 检查）走完——密码过期的账号才会把改密对话抛出来。用 exec 替换掉登录 shell，对话一结束
// 会话就结束，不留一个还在等输入的 shell。实测 M2/T1/T2 用的是同一形态。
const passwdCommand = "exec :"

// passwdNotExpired 整场没出现过期信号时的收尾文案（未过期机器）。
const passwdNotExpired = "密码未过期，跳过改密"

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
	result.Output = trimOuterBlankLines(out.String())

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

// passwdHook 改密的写侧钩子：采集整备交给 outputTap，匹配交给 passwdMatcher。
// 与代填的钩子同一形态，差别只在匹配语义——它只发「该喂什么」，没有中止词。
type passwdHook struct {
	tap     *outputTap
	matcher *passwdMatcher
	values  map[PasswdValue]string
	events  chan hookEvent
}

func newPasswdHook(out *lockedBuffer, in PasswdInput, currentPassword string) *passwdHook {
	h := &passwdHook{
		matcher: newPasswdMatcher(in.Prompts, in.EnterKeywords, in.Match),
		values: map[PasswdValue]string{
			PasswdValueCurrent: currentPassword,
			PasswdValueNew:     in.NewPassword,
		},
		// 容量按全表预算给：命中即发，钩子永不阻塞
		events: make(chan hookEvent, in.Prompts.stepBudget()+1),
	}
	h.tap = newOutputTap(out, h.match)
	return h
}

func (h *passwdHook) Write(p []byte) (int, error) { return h.tap.Write(p) }

func (h *passwdHook) Flush() { h.tap.Flush() }

// match 判定命中并发出该喂的值。过期信号在这里只开喂，不终止会话。
func (h *passwdHook) match(chunk string) {
	for _, v := range h.matcher.feed(chunk) {
		h.events <- hookEvent{kind: eventSend, text: h.values[v]}
	}
}

// sawEnter 整场是否出现过入场信号——收尾判定用，与钩子的写侧共用同一把锁。
func (h *passwdHook) sawEnter() bool {
	var seen bool
	h.tap.locked(func() { seen = h.matcher.sawEnterSignal() })
	return seen
}
