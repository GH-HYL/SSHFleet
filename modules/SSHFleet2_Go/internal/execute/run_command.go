package execute

// 跑一条命令：开会话、把正文经 stdin 喂下去、收输出、收退出码、按纸上那个超时收场。
//
// 只写事实：退出码、输出原文、超时与取消两个标志、报错原文。**「这一台算成功还是失败」
// 不在这一层**——分类由 `visit` 收齐事实之后统一定（本包 `verdict.go`），成败由结算站
// 拿分类解释（ADR-0035 决定四）。

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"

	"sshfleet2/internal/contract"
)

// lockedBuffer 带锁缓冲：stdout 与 stderr 合并写入、保持顺序。
//
// 两个流合并到一个缓冲是老版的口径：使用者看到的是一台机器的输出，不是两份交错的流；
// 顺序按到达先后，而到达先后已经是目标机实际写出的顺序能给出的最好近似。
type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (w *lockedBuffer) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.Write(p)
}

func (w *lockedBuffer) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.String()
}

// runCommand 在一台已有连接上跑这次的任务，把事实写进 res。
//
// 会话开不起来、拿不到输入通道：两件事分开报（老版同口径）——前者是目标机不给开会话，
// 后者是会话开了但 stdin 通道没拿到，排查方向不同。
func (c *client) runCommand(ctx context.Context, p *contract.Plan, res *contract.NodeResult) {
	session, err := c.conn.NewSession()
	if err != nil {
		res.ErrorText = c.mergeBanner(msg(envSession, sessionCreateFailed, err.Error()))
		return
	}
	defer func() { _ = session.Close() }()

	out := &lockedBuffer{}
	session.Stdout = out
	session.Stderr = out

	command, stdin := downCommand(*p.Task.Command, p.Run)
	if stdin != "" {
		w, werr := session.StdinPipe()
		if werr != nil {
			res.ErrorText = c.mergeBanner(msg(envSession, sessionChannelFailed, werr.Error()))
			return
		}
		go func() {
			_, _ = io.WriteString(w, stdin)
			_ = w.Close()
		}()
	}

	start := time.Now()
	err, timedOut, cancelled := runWithTimeout(ctx, session, command, seconds(p.Run.TimeoutExecute))
	res.Cost = time.Since(start)
	res.Output = trimOuterBlankLines(out.String())

	// 结束信号 → 事实字段（单点在 endFactsOf）。
	f := endFactsOf(err, timedOut, cancelled)
	res.ExitCode = f.exitCode
	res.TimedOut = f.timedOut
	res.Cancelled = f.cancelled
	// 服务端提示无条件附上：密码过期那类提示伴随的是成功的连接，分类要靠它认出来。
	res.ErrorText = c.mergeBanner(deref(f.errText))
}

// runWithTimeout 执行超时与外部取消两条路，后两个返回值把「到点收场 / 被喊停」
// 报给调用方写进事实标志——这两个事实不该再靠辨认报错文案反推。
//
// 超时经 context 表达：到点就关会话，远端那条命令随之收手。
func runWithTimeout(parent context.Context, session *ssh.Session, command string, timeout time.Duration) (err error, timedOut bool, cancelled bool) {
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- session.Run(command) }()

	select {
	case err = <-done:
		return err, false, false
	case <-ctx.Done():
		_ = session.Close()
		if parent.Err() != nil {
			// 外部喊停（Ctrl+C / 上级取消）：不是这台机器自己的毛病。
			return parent.Err(), false, true
		}
		return errors.New(msg(envRun, runTimeout, fmt.Sprintf("exceeded %v", timeout))), true, false
	}
}

// endFacts 一次执行的结束信号 → 事实字段。
//
//   - 干净收场（err==nil）     → 退出码 0
//   - 非超时非中断的异常        → 拿得到退出码就用它，拿不到就把远端原文记为报错
//   - 超时 / 中断              → 记原文，两个标志置真
//
// 顺序要紧：超时与中断同样在 err 上带一个错误值，先走非超时分支会把「到点」这件事说成
// 远端的原因。故由两个标志把关。
type endFacts struct {
	exitCode  *int
	errText   *string
	timedOut  bool
	cancelled bool
}

func endFactsOf(err error, timedOut, cancelled bool) endFacts {
	f := endFacts{timedOut: timedOut, cancelled: cancelled}
	switch {
	case err == nil:
		code := 0
		f.exitCode = &code
	case !timedOut && !cancelled:
		if code := extractExitCode(err); code != nil {
			f.exitCode = code
		} else if isExitMissing(err) {
			// 库已经说了「会话结束了、没收到退出状态」——这本身就是分类依据，
			// 翻成报文，不再退化成靠关键词匹配库提示原文。
			f.errText = strPtr(msg(envSession, sessionExitMissing, err.Error()))
		} else if isCommandRejected(err) {
			// 库已经说了「目标机拒了这次请求」：命令没跑，没有退出码可给。
			f.errText = strPtr(msg(envRun, runRejected, err.Error()))
		} else {
			f.errText = strPtr(err.Error())
		}
	default:
		f.errText = strPtr(err.Error())
	}
	return f
}

// extractExitCode 只有 *ssh.ExitError（命令以非 0 退出）带真实退出码；
// 超时、中断这类非命令执行错误没有码可给。
func extractExitCode(err error) *int {
	var exitErr *ssh.ExitError
	if errors.As(err, &exitErr) {
		code := exitErr.ExitStatus()
		return &code
	}
	return nil
}

// isExitMissing 库说「会话结束了、但没收到退出状态」。
func isExitMissing(err error) bool {
	var missing *ssh.ExitMissingError
	return errors.As(err, &missing)
}

// isCommandRejected 库说「目标机不接受这次会话 / 命令请求」。
//
// 这是「连上了、会话也开了，但对方拒绝执行」那一类：命令根本没跑，因此没有退出码，
// 也不能靠库的提示原文去认（措辞一变就静默失效）。
func isCommandRejected(err error) bool {
	var openErr *ssh.OpenChannelError
	return errors.As(err, &openErr)
}

// trimOuterBlankLines 远端输出写进 Output 前的整备：只去掉**最前与最后**的空白行，
// 中间原样——中间的空行与每行行首缩进都是输出自身的结构。
//
// 为什么不能 TrimSpace：那等于把「块首」当成「行首」——`who -b` 只有一行，行首的对齐
// 空格落在块首，会被当首部空白一起吃掉，看起来像「随机丢缩进」。
//
// 为什么先把 CR 归一：远端回 CRLF 或孤立 CR 是常态（`--no-shell` 原样下发时尤其如此）。
// CR 留到终端会被当成「回到行首」的控制指令，行中间那个会把行首覆盖成空白，孤立的那些
// 会把整行吃掉——屏幕上看着是空行，归档文件里字字都在。归一只管「行怎么断」，
// 不动任何可见字符，三处去向拿到的是同一份已经断好行的文本。
func trimOuterBlankLines(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	lines := strings.Split(s, "\n")
	start, end := 0, len(lines)
	for start < end && strings.TrimSpace(lines[start]) == "" {
		start++
	}
	for end > start && strings.TrimSpace(lines[end-1]) == "" {
		end--
	}
	return strings.Join(lines[start:end], "\n")
}

func strPtr(s string) *string { return &s }

func deref(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
