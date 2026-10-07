package ssh

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

// 交互分支：远端程序要向操作者索要输入时（选架构、选 deb/rpm 等），按预置的对应关系
// 自动填进去。与普通路径的差别只有三处——分配 PTY、正文改经命令行承载、会话 stdin
// 整条让给代填；其余（结果字段、错误文案、超时与中断语义）与普通路径同源。
//
// 为什么要独立文件：内容承载形态与普通路径不同，按《个人开发规范》二，新环节新增
// 实现文件，不给 RunCommand 加分支。

// PTY 尺寸：200 列 × 50 行，不继承本地终端尺寸——本地可能是窄窗口，
// 远端按列宽自排版时会把提示串折行切断，触发词就永远匹配不上。
const (
	ptyTerm = "xterm"
	ptyRows = 50
	ptyCols = 200
)

// answerEOL 送出代填时补的回车。PTY 下 read/select 收不到换行不返回，
// 故「代填内容 + 一个 \n」是唯一能表达「输入完毕」的形态（第一段留空 = 只发一个回车）。
const answerEOL = "\n"

// Answer 一条代填：填进远端输入通道的内容 + 认出提问的触发词。
type Answer struct {
	Value    string
	Triggers []string
}

// Describe 代填的呈现形态（报告与参数屏共用）：内容 → 触发词。
// 内容留空是合法写法（只发一个回车），这里按原义报出来，不显示成空。
func (a Answer) Describe() string {
	value := a.Value
	if value == "" {
		value = "（回车）"
	}
	return value + " → " + strings.Join(a.Triggers, "、")
}

// InteractiveInput 交互分支的入场值：正文、解释器、身份标记，加代填表、匹配口径与中止词。
// 与 DescribeInput 同一手法——只收真正需要的几项，不收整份命令行参数。
type InteractiveInput struct {
	Command       string   // 命令模式：命令原文
	ScriptBody    string   // 脚本模式：脚本正文（非空即脚本模式）
	ScriptName    string   // 脚本模式：脚本文件名（下发行把它交回去当 $0，见 scriptNameArg）
	Interpreter   string   // 解释器：命令模式是命令解释器，脚本模式是脚本解释器
	AsRoot        bool     // -m sudo
	Answers       []Answer // 代填表（顺序即命令行给出顺序）
	Match         MatchOptions
	AbortKeywords []string // 中止词：关键词取判据文件「密码过期」分类
}

// InteractiveCommand 拼交互分支的下发行：正文 base64 编入命令行，远端 base64 -d 后
// 作内层解释器的「程序文本参数」。长度检查与实际执行都调它，不两处各拼一份。
//
// 「程序文本参数」各解释器不同（shell / python 是 -c，perl / ruby / node / lua 是 -e，
// php 是 -r），由 ProgramTextArg 按解释器取；表外的解释器在参数合规检查阶段已被拦下，
// 这里兜底用 -c。
//
// 脚本模式还会把脚本文件名作参数之后的第一个参数交回去——那正是 shell 的 $0，
// 脚本据此仍能从自己的名字里取信息（理由见 scriptNameArg）。命令模式没有文件名，不补。
//
// 为什么不走 stdin：一次会话只有一条 stdin，代填要独占它；两者共用时远端 read 会吃掉
// 脚本体自己的下一行，后半段静默消失且不报错（实测 F4/F5）。为什么不落盘：目标机
// 磁盘满是常态，落 /tmp 会因磁盘失败（ADR-0010）。
func InteractiveCommand(in InteractiveInput) string {
	body := in.Command
	if in.ScriptBody != "" {
		body = in.ScriptBody
	}
	encoded := base64.StdEncoding.EncodeToString([]byte(body))
	programArg, ok := ProgramTextArg(in.Interpreter)
	if !ok {
		programArg = "-c" // 合规检查已拦下表外的解释器，这里兜底，不该发生
	}
	inner := innerCommand(in.Interpreter, in.AsRoot) + " " + programArg +
		` "$(printf %s ` + encoded + ` | base64 -d)"`
	if in.ScriptBody != "" { // 命令模式没有脚本文件，不补名字
		inner += scriptNameArg(in.Interpreter, in.ScriptName)
	}
	return "sh -c " + shellQuote(inner)
}

// ScriptBodyOf 脚本文件内容 → 会话里真正执行的正文：剥 UTF-8 BOM、去掉整块首尾空白。
//
// BOM 不剥会跟着内容进远端 bash：第一行变成带 BOM 的命令，bash 报
// "No such file or directory"、那一行废掉（清单侧同口径，见 nodelist.csvread）。
// 抽成一处的原因：长度检查与实际执行都要这份正文，两处各整备一次就会算出不同的长度。
func ScriptBodyOf(data []byte) string {
	return strings.TrimSpace(strings.TrimPrefix(string(data), "\ufeff"))
}

// DescribeInteractiveInput DescribeInteractive 的入参：只收打印真正需要的几项。
type DescribeInteractiveInput struct {
	Command    string // 命令模式：命令原文
	ScriptPath string // 脚本模式：脚本文件路径
	ScriptBody string // 脚本模式：脚本正文（非空即判定为脚本模式）
	Answers    int    // 代填条数
	Downlink   string // 下发行（由 InteractiveCommand 单点生成）
}

// DescribeInteractive 交代交互分支「交给 SSH 执行的是什么」——供日志打印，不参与下发。
//
// 三段「原始 → 处理方式 → 最终命令」对位旧 Py 版 builder.py 的日志形态：正文一进
// base64，日志里就认不出它是什么了，所以必须把原文一并交代。
func DescribeInteractive(a DescribeInteractiveInput) string {
	const head = "交给 SSH 执行：\n"
	tail := "  最终命令： " + a.Downlink
	if a.ScriptBody != "" {
		return head +
			"  原始脚本： " + a.ScriptPath + "（正文经 base64 编入命令行，不落盘）\n" +
			"  处理方式： 会话 stdin 整条让给代填（" + strconv.Itoa(a.Answers) + " 条）\n" +
			tail
	}
	return head +
		"  原始命令： " + a.Command + "\n" +
		"  处理方式： 正文经 base64 编入命令行（不落盘），会话 stdin 整条让给代填（" +
		strconv.Itoa(a.Answers) + " 条）\n" +
		tail
}

// RunInteractive 执行命令/脚本，并按代填表自动回应远端的索要输入。
//
// 与 RunCommand 复用同一套收尾：newResult / connectFor / captureBanner / lockedBuffer /
// trimOuterBlankLines / extractExitCode；超时与中断的契约也照抄（超时返回
// 「命令执行超时(%v)」、外部取消返回 parent.Err()）。
//
// 只写事实字段，不写结论：未命中进 AnswersMissed、中止词原文进 AbortLine、
// 表已空仍超时置 AnswersExhausted——成败与分类由结果判定产生，这里不再抹退出码、
// 不再拼"触发词未命中：…"之类的结论文案。
func (c *Client) RunInteractive(ctx context.Context, in InteractiveInput, seq int) *Result {
	result := c.newResult(seq)
	defer c.captureBanner(result)

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

	// tty 模式传 nil：用远端默认（onlcr、echo 都照开）。回显也在采集范围内，
	// 故触发词要有区分度（配置纪律，不是代码能兜住的）。
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
	hook := newOutputHook(out, in)
	session.Stdout = hook
	session.Stderr = hook

	// 送内容放在自己的协程里：写输入通道在窗口满时会阻塞，不能因此卡住消费循环
	// （消费一停，x/crypto 的复制协程就停，远端输出回压），也不能挡住 ctx 到点。
	send := make(chan string, len(in.Answers)+1)
	go func() {
		for text := range send {
			_, _ = io.WriteString(stdin, text+answerEOL)
		}
	}()

	execStart := time.Now()
	outcome := c.runInteractiveSession(ctx, session, hook, send, InteractiveCommand(in))
	result.ExecCostTime = time.Since(execStart).Seconds()
	close(send)
	hook.Flush()
	result.Output = trimOuterBlankLines(out.String())

	// 事实：会话起没起、怎么收的场、目的命令的退出码
	result.SessionBegun = outcome.begun
	f := endFactsOf(outcome.err, outcome.timedOut, outcome.canceled)
	result.CommandExitCode = f.exitCode
	result.Error = f.errText
	result.TimedOut = f.timedOut
	result.Canceled = f.canceled

	// 事实：代填的收尾三态。中止词优先（命中过的场次不判未命中，否则即时归因会被盖掉）；
	// 中断有自己的去处（任务已取消），也不判未命中。
	missed := hook.pending()
	result.AnswersMissed = missed
	if outcome.abortLine != "" {
		result.AbortLine = outcome.abortLine
	} else if outcome.begun && !outcome.canceled && len(missed) == 0 &&
		outcome.timedOut && len(in.Answers) > 0 {
		// 表已送空仍超时：工具分不出「脚本卡在没配触发词的提示上」与「任务本身耗时长」，
		// 但「表已空」是确定的事实——判定按它给「代填用尽后超时」。
		result.AnswersExhausted = true
	}
	return result
}

// interactiveOutcome 会话结束的样子：错误值 + 收尾判定要用的事实。
type interactiveOutcome struct {
	err       error
	begun     bool   // 命令是否真的下发成功（收尾判定的前提）
	canceled  bool   // 外部取消（中断）
	timedOut  bool   // 到点收场（-t）
	aborted   bool   // 被中止过：代填的中止词命中，或改密的「无话可给」
	abortLine string // 代填中止词命中的原文行（改密不收 text，故这里为空——判「有没有中止」看 aborted）
}

// sessionDriver 会话驱动要的三件：会话本体、事件源、送信通道。
type sessionDriver struct {
	session *ssh.Session
	events  <-chan hookEvent
	send    chan<- string
}

// driveSession 边跑边喂：Start + 事件循环 + Wait。代填与改密共用这一份。
//
// 必须用 Start：Run 等价于 Start + Wait，会占住调用方直到命令结束，没有机会边跑边填。
// 超时与中断自带一份与 runWithTimeoutAndCancel 等价的包装（既有函数一行不改）。
//
// 事件只有两种：该喂什么（eventSend）、中止词命中（eventAbort，只有代填会发）。
func (c *Client) driveSession(parent context.Context, d sessionDriver, command string) interactiveOutcome {
	ctx, cancel := context.WithTimeout(parent, c.cfg.ExecTimeout)
	defer cancel()

	started := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		if err := d.session.Start(command); err != nil {
			done <- err // 命令未能开始：交由调用方保留错误原文
			return
		}
		close(started)
		done <- d.session.Wait()
	}()

	var out interactiveOutcome
	for {
		select {
		case <-started:
			started = nil // 已下发，不再关注（nil 通道恒阻塞）
			out.begun = true

		case ev := <-d.events:
			switch ev.kind {
			case eventSend:
				d.send <- ev.text
			case eventAbort:
				out.aborted = true
				out.abortLine = ev.text
				_ = d.session.Close() // 立即终止该节点会话
				return out
			}

		case err := <-done:
			out.err = err
			return out

		case <-ctx.Done():
			_ = d.session.Close()
			if parent.Err() != nil {
				out.err = parent.Err() // 外部取消（中断 / 上级超时）
				out.canceled = true
				return out
			}
			out.err = fmt.Errorf("命令执行超时(%v)", c.cfg.ExecTimeout)
			out.timedOut = true
			return out
		}
	}
}

// runInteractiveSession 代填的会话驱动：把写侧钩子接上共用驱动。
func (c *Client) runInteractiveSession(parent context.Context, session *ssh.Session, hook *outputHook, send chan<- string, command string) interactiveOutcome {
	return c.driveSession(parent, sessionDriver{session: session, events: hook.events, send: send}, command)
}

// 写侧钩子发出的事件：命中一条代填该送的内容 / 中止词命中。
const (
	eventSend = iota
	eventAbort
)

type hookEvent struct {
	kind int
	text string
}

// outputHook 代填的写侧钩子：x/crypto 的复制协程把远端输出推到这里（stdout 与 stderr 同一个）。
//
// 采集整备（去行尾 `\r` → 落 lockedBuffer）交给 outputTap，本结构只管代填的匹配语义。
// 只判定、只发信号：送内容与关会话都由调用方那侧收口（跨协程关 channel 的风险没必要冒）。
type outputHook struct {
	tap     *outputTap
	values  []string
	answers *cursorMatcher
	abort   *cursorMatcher
	events  chan hookEvent
}

func newOutputHook(out *lockedBuffer, in InteractiveInput) *outputHook {
	rules := make([][]string, 0, len(in.Answers))
	values := make([]string, 0, len(in.Answers))
	for _, a := range in.Answers {
		rules = append(rules, a.Triggers)
		values = append(values, a.Value)
	}
	h := &outputHook{
		values:  values,
		answers: newCursorMatcher(rules, in.Match),
		events:  make(chan hookEvent, len(in.Answers)+2), // 容量够放下全部命中，钩子永不阻塞
	}
	if len(in.AbortKeywords) > 0 {
		h.abort = newCursorMatcher([][]string{in.AbortKeywords}, in.Match)
	}
	h.tap = newOutputTap(out, h.match)
	return h
}

func (h *outputHook) Write(p []byte) (int, error) { return h.tap.Write(p) }

func (h *outputHook) Flush() { h.tap.Flush() }

// pending 还没送出的代填条目序号（1 起）——收尾判定用，与钩子的写侧共用同一把锁。
func (h *outputHook) pending() []int {
	var out []int
	h.tap.locked(func() { out = h.answers.pending() })
	return out
}

// match 判定命中并发信号。中止词优先：命中即终止，同块里的代填不再送出。
func (h *outputHook) match(chunk string) {
	if h.abort != nil {
		if hits := h.abort.feed(chunk); len(hits) > 0 {
			h.events <- hookEvent{kind: eventAbort, text: hits[0].line}
			return
		}
	}
	for _, hit := range h.answers.feed(chunk) {
		h.events <- hookEvent{kind: eventSend, text: h.values[hit.rule]}
	}
}
