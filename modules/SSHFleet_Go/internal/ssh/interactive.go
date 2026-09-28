package ssh

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
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
	Interpreter   string   // 脚本模式：bash / python3
	AsRoot        bool     // -m sudo
	Answers       []Answer // 代填表（顺序即命令行给出顺序）
	Match         MatchOptions
	AbortKeywords []string // 中止词：关键词取判据文件「密码过期」分类
}

// InteractiveCommand 拼交互分支的下发行：正文 base64 编入命令行，远端 base64 -d 后
// 作内层解释器的 -c 参数。长度检查与实际执行都调它，不两处各拼一份。
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
	inner := loginInner(in.ScriptBody != "", in.Interpreter, in.AsRoot) +
		` -c "$(printf %s ` + encoded + ` | base64 -d)"`
	return "bash -lc " + shellQuote(inner)
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
// 与 RunCommand 复用同一套收尾：newResult / connectFor / applyBanner / lockedBuffer /
// trimOuterBlankLines / extractExitCode；超时与中断的契约也照抄（超时返回
// 「命令执行超时(%v)」、外部取消返回 parent.Err()）。
func (c *Client) RunInteractive(ctx context.Context, in InteractiveInput, seq int) *Result {
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

	// 收尾静态判定：命令已开始、没被中断、也没被中止词终止时，仍有代填没送出即为未命中。
	// 中断有自己的分类（任务已取消），中止词命中过的也不判——否则即时归因会被兜底分类盖掉。
	missed := hook.pending()
	if outcome.begun && !outcome.canceled && outcome.abortLine == "" {
		switch {
		case len(missed) > 0:
			result.Error = strPtr(missText(in.Answers, missed))
			result.ExitCode = nil
		case outcome.timedOut:
			// 表已送空仍超时：工具分不出「脚本卡在没配触发词的提示上」与「任务本身耗时长」，
			// 但「表已空」是确定的事实——补进超时文案，判据文件另有一条分类接住它。
			result.Error = strPtr(fmt.Sprintf("%s；本次代填 %d 条已全部送出", outcome.err, len(in.Answers)))
		}
	}

	// 中止词命中的节点：error 写命中的原文行、退出码置 nil → 分类「密码过期」。
	// 排在未命中判定之后，两条不会同时成立（上面已排除 abortLine 非空的情形）。
	if outcome.abortLine != "" {
		result.Error = strPtr(outcome.abortLine)
		result.ExitCode = nil
	}
	return result
}

// interactiveOutcome 会话结束的样子：错误值 + 收尾判定要用的事实。
type interactiveOutcome struct {
	err       error
	begun     bool   // 命令是否真的下发成功（收尾判定的前提）
	canceled  bool   // 外部取消（中断）
	timedOut  bool   // 到点收场（-t）
	abortLine string // 中止词命中的原文行（空 = 未命中）
}

// runInteractiveSession 边跑边代填：Start + 事件循环 + Wait。
//
// 必须用 Start：Run 等价于 Start + Wait，会占住调用方直到命令结束，没有机会边跑边填。
// 超时与中断自带一份与 runWithTimeoutAndCancel 等价的包装（既有函数一行不改）。
func (c *Client) runInteractiveSession(parent context.Context, session *ssh.Session, hook *outputHook, send chan<- string, command string) interactiveOutcome {
	ctx, cancel := context.WithTimeout(parent, c.cfg.ExecTimeout)
	defer cancel()

	started := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		if err := session.Start(command); err != nil {
			done <- err // 命令未能开始：交由调用方保留错误原文
			return
		}
		close(started)
		done <- session.Wait()
	}()

	var out interactiveOutcome
	for {
		select {
		case <-started:
			started = nil // 已下发，不再关注（nil 通道恒阻塞）
			out.begun = true

		case ev := <-hook.events:
			switch ev.kind {
			case eventSend:
				send <- ev.text
			case eventAbort:
				out.abortLine = ev.text
				_ = session.Close() // 立即终止该节点会话
				return out
			}

		case err := <-done:
			out.err = err
			return out

		case <-ctx.Done():
			_ = session.Close()
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

// missText 触发词未命中的收尾文案：带未命中的序号与触发词原文——诊断时要看的就是它。
// 脚本是顺序提问的，所以序号里最小的那个就是第一个出错点。
func missText(answers []Answer, missed []int) string {
	nums := make([]string, 0, len(missed))
	words := make([]string, 0, len(missed))
	for _, n := range missed {
		nums = append(nums, strconv.Itoa(n))
		words = append(words, `"`+strings.Join(answers[n-1].Triggers, "、")+`"`)
	}
	return fmt.Sprintf("触发词未命中：第 %s 条（%s）未匹配到任何输出",
		strings.Join(nums, "、"), strings.Join(words, "、"))
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

// outputHook 写侧钩子：x/crypto 的复制协程把远端输出推到这里（stdout 与 stderr 同一个）。
//
// 按序做三件：去行尾 \r → 写进 lockedBuffer（Output 字段与普通路径同源）→ 喂匹配器。
// 去 \r 放在入口——缓冲与匹配器必须看到同一份字节，游标才有意义。
// 只判定、只发信号：送内容与关会话都由调用方那侧收口（跨协程关 channel 的风险没必要冒）。
type outputHook struct {
	mu      sync.Mutex
	out     *lockedBuffer
	held    string // 待定的行尾 \r：与紧跟的 \n 可能被切在相邻两块里
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
		out:     out,
		values:  values,
		answers: newCursorMatcher(rules, in.Match),
		events:  make(chan hookEvent, len(in.Answers)+2), // 容量够放下全部命中，钩子永不阻塞
	}
	if len(in.AbortKeywords) > 0 {
		h.abort = newCursorMatcher([][]string{in.AbortKeywords}, in.Match)
	}
	return h
}

// Write 采集侧整备 + 匹配（stdout 与 stderr 由两个复制协程推来，故带锁）。
func (h *outputHook) Write(p []byte) (int, error) {
	h.mu.Lock()
	defer h.mu.Unlock()

	chunk := h.held + string(p)
	h.held = ""
	// 末尾的 \r 先留着：它后面可能跟一个 \n（被切在下一块里），去不去要等下一块到齐
	if strings.HasSuffix(chunk, "\r") {
		chunk, h.held = chunk[:len(chunk)-1], "\r"
	}
	h.emit(strings.ReplaceAll(chunk, "\r\n", "\n"))
	return len(p), nil
}

// Flush 把待定的 \r 交给缓冲与匹配器（会话结束后调用一次）。
func (h *outputHook) Flush() {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.held == "" {
		return
	}
	chunk := h.held
	h.held = ""
	h.emit(chunk)
}

// pending 还没送出的代填条目序号（1 起）——收尾判定用，与钩子的写侧共用同一把锁。
func (h *outputHook) pending() []int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.answers.pending()
}

// emit 同一份字节先落缓冲、再喂匹配器。
func (h *outputHook) emit(chunk string) {
	if chunk == "" {
		return
	}
	_, _ = h.out.Write([]byte(chunk))
	h.match(chunk)
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
