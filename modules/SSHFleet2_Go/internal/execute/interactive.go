package execute

// 交互那一套：PTY 会话驱动、采集侧钩子、触发词匹配。
//
// 代填与改密共用底层三件（PTY 尺寸、写侧钩子、Start + 事件循环），差别在匹配语义与
// 中止条件——代填是「触发词命中就填、命中中止词就停」，改密是「出现入场信号才开始喂、
// 表喂满还在问就收手」。方向恰好相反，所以各一个文件。

import (
	"context"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"

	"sshfleet2/internal/contract"
)

// PTY 尺寸：200 列 × 50 行，不继承本地终端尺寸——本地可能是窄窗口，远端按列宽自排版时
// 会把提示串折行切断，触发词就永远匹配不上。
const (
	ptyTerm = "xterm"
	ptyRows = 50
	ptyCols = 200
)

// answerEOL 送出时补的回车。PTY 下 read/select 收不到换行不返回，
// 故「内容 + 一个 \n」是唯一能表达「输入完毕」的形态（内容留空 = 只发一个回车）。
const answerEOL = "\n"

// MatchOptions 触发词与中止词的匹配口径（配置 `[interactive]` 的两个开关，随纸来）。
type MatchOptions struct {
	Regex         bool
	CaseSensitive bool
}

// patternOf 关键词 → 正则：不区分大小写用内联标记表达，不对正文做大小写转换
// （转换会改变个别非 ASCII 字符的字节长度，而游标是按字节推进的）。
func patternOf(keyword string, caseSensitive bool) string {
	if caseSensitive {
		return keyword
	}
	return "(?i)" + keyword
}

// keywordRule 一组关键词：一条代填的触发词，或整组中止词。
type keywordRule struct {
	keywords []string
	patterns []*regexp.Regexp // 正则模式下与 keywords 对位；子串模式下为空
}

func newKeywordRule(keywords []string, opts MatchOptions) keywordRule {
	r := keywordRule{keywords: keywords}
	if opts.Regex {
		r.patterns = make([]*regexp.Regexp, 0, len(keywords))
		for _, kw := range keywords {
			r.patterns = append(r.patterns, regexp.MustCompile(patternOf(kw, opts.CaseSensitive)))
		}
	}
	return r
}

// find 在 s 里找最先出现的关键词，返回起点、终点与命中的那一条。
func (r keywordRule) find(s string, opts MatchOptions) (int, int, string, bool) {
	bestStart, bestEnd, bestKeyword := -1, -1, ""
	for i, kw := range r.keywords {
		start, end := -1, -1
		switch {
		case opts.Regex:
			if idx := r.patterns[i].FindStringIndex(s); idx != nil {
				start, end = idx[0], idx[1]
			}
		default:
			if idx := indexFold(s, kw, opts.CaseSensitive); idx >= 0 {
				start, end = idx, idx+len(kw)
			}
		}
		// 零宽命中（如正则 a*）不推进游标，会原地死循环；它也不可能是提示文本，直接不要。
		if start < 0 || end <= start {
			continue
		}
		if bestStart < 0 || start < bestStart {
			bestStart, bestEnd, bestKeyword = start, end, kw
		}
	}
	if bestStart < 0 {
		return 0, 0, "", false
	}
	return bestStart, bestEnd, bestKeyword, true
}

// indexFold 子串包含查找；不区分大小写时只折叠 ASCII 大小写。
//
// 不用 strings.ToLower：个别非 ASCII 字符转换后会变字节长度，而游标按字节推进，
// 长度一变游标就错位。
func indexFold(s, keyword string, caseSensitive bool) int {
	if caseSensitive || keyword == "" {
		return strings.Index(s, keyword)
	}
	return strings.Index(lowerASCII(s), lowerASCII(keyword))
}

// lowerASCII 只折叠 ASCII 大写字母；无大写时原样返回。
func lowerASCII(s string) string {
	var b []byte
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c < 'A' || c > 'Z' {
			continue
		}
		if b == nil {
			b = []byte(s)
		}
		b[i] = c + ('a' - 'A')
	}
	if b == nil {
		return s
	}
	return string(b)
}

// cursorMatcher 游标匹配器：多条规则并行参与，共用一个游标，命中即推进到触发词末尾。
//
// 为什么必须共用游标：读到的字节流可能只含半个提示，游标不动就能等后半截到齐；
// 命中后推进到触发词末尾而不是段末——同一次推来的块里可能连着两条提示。
type cursorMatcher struct {
	rules     []keywordRule
	opts      MatchOptions
	tail      string
	done      []bool
	remaining int
}

func newCursorMatcher(rules [][]string, opts MatchOptions) *cursorMatcher {
	m := &cursorMatcher{opts: opts, remaining: len(rules), done: make([]bool, len(rules))}
	for _, keywords := range rules {
		m.rules = append(m.rules, newKeywordRule(keywords, opts))
	}
	return m
}

// hit 一次命中：规则序号、命中的关键词原文、命中所在行的原文。
type hit struct {
	rule    int
	keyword string
	line    string
	end     int
}

// feed 追加新到的字节，返回本次新增的命中（按出现先后）。
func (m *cursorMatcher) feed(chunk string) []hit {
	if m.remaining == 0 || len(m.rules) == 0 {
		return nil
	}
	m.tail += chunk

	var hits []hit
	for {
		h, ok := m.next()
		if !ok {
			break
		}
		hits = append(hits, h)
		m.done[h.rule] = true
		m.remaining--
		m.tail = m.tail[h.end:]
	}
	if m.remaining == 0 {
		m.tail = ""
	}
	return hits
}

// next 在游标之后找最先出现的命中：多条规则并行，取位置最靠前者。
func (m *cursorMatcher) next() (hit, bool) {
	best := hit{rule: -1}
	bestStart := -1
	for i, rule := range m.rules {
		if m.done[i] {
			continue
		}
		start, end, keyword, ok := rule.find(m.tail, m.opts)
		if !ok || (bestStart >= 0 && start >= bestStart) {
			continue
		}
		bestStart = start
		best = hit{rule: i, keyword: keyword, line: lineAt(m.tail, start, end), end: end}
	}
	if bestStart < 0 {
		return hit{}, false
	}
	return best, true
}

// pending 还没命中的规则序号（1 起，按给定顺序）。
func (m *cursorMatcher) pending() []int {
	var out []int
	for i, done := range m.done {
		if !done {
			out = append(out, i+1)
		}
	}
	return out
}

// lineAt 命中所在行的原文：该行行首到行尾，不含换行与行尾 \r。
func lineAt(s string, start, end int) string {
	from := strings.LastIndexByte(s[:start], '\n') + 1
	to := len(s)
	if idx := strings.IndexByte(s[end:], '\n'); idx >= 0 {
		to = end + idx
	}
	return strings.TrimSuffix(s[from:to], "\r")
}

// —— 采集侧 ——

// outputTap 采集侧的共同前半段（代填与改密共用）。
//
// session.Stdout 与 session.Stderr 各由一个复制协程推来，所以写入要带锁；结尾的 `\r`
// 先留住——它后面可能跟一个 `\n`，而两者会被切在相邻两块里。整备好的**同一份**字节既落
// 缓冲、又交给 onChunk：缓冲与匹配器必须看到同一份字节，游标才有意义。
type outputTap struct {
	mu      sync.Mutex
	out     *lockedBuffer
	held    string
	onChunk func(string)
}

func newOutputTap(out *lockedBuffer, onChunk func(string)) *outputTap {
	return &outputTap{out: out, onChunk: onChunk}
}

func (t *outputTap) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.appendLocked(string(p))
	return len(p), nil
}

func (t *outputTap) Flush() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.held == "" {
		return
	}
	chunk := t.held
	t.held = ""
	t.emitLocked(chunk)
}

func (t *outputTap) locked(fn func()) {
	t.mu.Lock()
	defer t.mu.Unlock()
	fn()
}

func (t *outputTap) appendLocked(s string) {
	chunk := t.held + s
	t.held = ""
	if strings.HasSuffix(chunk, "\r") {
		chunk, t.held = chunk[:len(chunk)-1], "\r"
	}
	t.emitLocked(strings.ReplaceAll(chunk, "\r\n", "\n"))
}

func (t *outputTap) emitLocked(chunk string) {
	if chunk == "" {
		return
	}
	_, _ = t.out.Write([]byte(chunk))
	t.onChunk(chunk)
}

// —— 会话驱动 ——

// 写侧钩子发出的事件。
const (
	eventSend = iota
	eventAbort
)

type hookEvent struct {
	kind int
	text string
}

// interactiveOutcome 会话结束的样子：错误值 + 收尾判定要用的事实。
type interactiveOutcome struct {
	err       error
	begun     bool
	canceled  bool
	timedOut  bool
	aborted   bool
	abortLine string
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
func (c *client) driveSession(parent context.Context, d sessionDriver, command string, timeout time.Duration) interactiveOutcome {
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()

	started := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		if err := d.session.Start(command); err != nil {
			done <- err
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
				out.err = parent.Err()
				out.canceled = true
				return out
			}
			out.err = errors.New(msg(envRun, runTimeout, fmt.Sprintf("exceeded %v", timeout)))
			out.timedOut = true
			return out
		}
	}
}

// startPTY 开一个会话并分配终端，交回会话与输入通道。
//
// tty 模式传 nil：用远端默认（onlcr、echo 都照开）。回显也在采集范围内，
// 故触发词要有区分度（配置纪律，不是代码能兜住的）。
func (c *client) startPTY() (*ssh.Session, io.WriteCloser, error) {
	session, err := c.conn.NewSession()
	if err != nil {
		return nil, nil, errors.New(msg(envSession, sessionCreateFailed, err.Error()))
	}
	if err := session.RequestPty(ptyTerm, ptyRows, ptyCols, nil); err != nil {
		_ = session.Close()
		return nil, nil, errors.New(msg(envSession, sessionPtyFailed, err.Error()))
	}
	stdin, err := session.StdinPipe()
	if err != nil {
		_ = session.Close()
		return nil, nil, errors.New(msg(envSession, sessionChannelFailed, err.Error()))
	}
	return session, stdin, nil
}

// pump 把该送的内容写到输入通道里（自己的协程：写通道在窗口满时会阻塞，
// 不能因此卡住消费循环，也不能挡住 ctx 到点）。
func pump(send <-chan string, stdin io.WriteCloser) {
	for text := range send {
		_, _ = io.WriteString(stdin, text+answerEOL)
	}
}

// stepsOf 记一条中间步骤的退出码：跑通记 0，失败记它自己的码（拿不到时留 nil）。
//
// 成功那一次也要记：`nil` 的语义是「没拿到码」（超时、中断、压根没走到那一步），
// 与「跑过、退出码 0」不是一回事，报告与日志要分得清。
func stepsOf(res *contract.NodeResult, name string, err error) {
	var code *int
	if err == nil {
		zero := 0
		code = &zero
	} else {
		code = extractExitCode(err)
	}
	res.Steps = append(res.Steps, contract.Step{Name: name, ExitCode: code})
}
