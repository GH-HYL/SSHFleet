package ssh

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
)

// lockedBuffer 带锁缓冲：stdout 与 stderr 合并写入、保持顺序（旧 threadSafeWriter）。
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

// newResult 单节点结果的共同开头：四条执行路径（命令 / 脚本 / 上传 / 下载）
// 都从这里起手，避免各写一份 Seq / IP / Port / User 的填充。
func (c *Client) newResult(seq int) *Result {
	return &Result{
		Seq: seq, IP: c.cfg.IP, Port: c.cfg.Port, User: c.cfg.User,
		AuthMethod: c.authMethodDesc(),
	}
}

// connectFor 建连并落定结果里的连接字段（成功与否、耗时、错误原文、认证失败分类）。
// 失败时返回 false——调用方据此直接返回该结果，不必各写一遍收尾。
// 成功后连接已建立，调用方负责 defer Close。
func (c *Client) connectFor(ctx context.Context, result *Result) bool {
	start := time.Now()
	err := c.Connect(ctx)
	result.ConnectCostTime = time.Since(start).Seconds()
	// 登录方式到这一刻才定得下来：密钥还是密码由实际挂进认证列表的方式决定。
	result.AuthMethod = c.authMethodDesc()
	if err != nil {
		result.ConnectSuccess = false
		// 这次建连失败是被外部中断打断的，不是目标机自己的毛病——照实记下来，并说成一句
		// 「连接还没建成就被中断」的报文（与执行 / 代填 / 改密 / 传输四条通道同一个口径）。
		// ctx 只由信号取消（main 里 signal.NotifyContext，无 deadline），故 Err() 非空
		// 就一定是"用户按了 Ctrl+C / 收到了 TERM"。
		if ctx.Err() != nil {
			result.Canceled = true
			result.Error = strPtr(AppendMessage(err.Error(), AbortMessage(true, false)))
		} else {
			result.Error = strPtr(err.Error())
		}
		return false
	}
	result.ConnectSuccess = true
	return true
}

// RunCommand 连接并执行一条命令/脚本（stdin 为空表示不喂输入）。
// 超时与中断：执行超时经 context.WithTimeout 表达（spec M3 差异，
// 旧实现是 select + time.After 建一个不会被取消的 timer）。
//
// 只写事实字段，不写结论（成败与分类由结果判定产生）：退出码进 CommandExitCode
// （目的那条命令，D33），结果退出码 ExitCode 一个字都不写。
func (c *Client) RunCommand(ctx context.Context, command, stdin string, seq int) *Result {
	result := c.newResult(seq)
	defer c.captureBanner(result) // 服务端提示无条件写独立字段（D34）

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

	out := &lockedBuffer{}
	session.Stdout = out
	session.Stderr = out

	if stdin != "" {
		w, werr := session.StdinPipe()
		if werr != nil {
			result.Error = strPtr(Message(EnvSession, sessionChannelFailed, werr.Error()))
			return result
		}
		go func() {
			_, _ = io.WriteString(w, stdin)
			_ = w.Close()
		}()
	}

	execStart := time.Now()
	err, timedOut, canceled := c.runWithTimeoutAndCancel(ctx, session, command)
	result.ExecCostTime = time.Since(execStart).Seconds()
	result.Output = trimOuterBlankLines(out.String())

	// 结束信号 → 事实字段（单点在 endFactsOf，与代填 / 改密通道共用）。
	f := endFactsOf(err, timedOut, canceled)
	result.CommandExitCode = f.exitCode
	result.Error = f.errText
	result.TimedOut = f.timedOut
	result.Canceled = f.canceled
	// 会话建立：拿到退出码、干净收场、或超时/中断（三者都只在命令已在跑时发生）。
	// 都不是（如远端拒了命令本身）即未下发。
	result.SessionBegun = f.exitCode != nil || f.timedOut || f.canceled
	// 收尾时说清这一摊是怎么结束的：被中断。分类照旧由判据表认领。
	if result.Canceled {
		result.Error = appendAbort(result.Error, true)
	}
	return result
}

// trimOuterBlankLines 远端输出写进 Output 字段前的整备：只去掉**最前与最后**的空白行，
// 中间原样——中间的空行、每行的行首缩进，都是命令输出自身的结构。
//
// 为什么只在这里做一次：Output 是原始字段，采集侧给出来就是干净的。终端与 output.txt
// 拿它直接输出、不做任何处理；xlsx 那边只额外清非法字符（见 output.cleanForExcel）。
// 整备若散到各输出端，就会各清各的、行为互相漂移。
//
// 为什么不能 TrimSpace：那等于把「块首」当成「行首」。`who -b` 只有一行，它的行首缩进
// 正好落在块首，会被当作首部空白一起吃掉——而那是输出自身的排版（`system boot` 前
// 那串空格是对齐用的）。多行输出只有第一行受害，症状看起来像「随机丢缩进」。
//
// 为什么不能逐行丢弃空白行：空行同样可能是输出的组成部分（`ls -l` 分组、`df -h`、
// 日志分段都靠空行分隔），按行清理会改掉原输出的结构。
//
// 为什么第一步要归一回车（2026-10-20 真机取证补上的一步）：远端回 CRLF 或孤立 CR 是常态
// ——`--no-shell` 原样下发、远端不经 tty 时，程序回什么字节就原样到达。CR 一旦留到终端，
// 就被当成「回到行首」的控制指令：行中间的那一个会把行首覆盖成空白，孤立的那些会把整行
// 吃掉——屏幕上只剩分隔线、看起来是空行，而归档文件里字字都在（同一次运行、两个去向对同一
// 段字节解释不同）。在这之前 xlsx 侧自己做了这层替换（cleanForExcel），终端与 txt 没有。
//
// 归一放在采集侧的这里，只管「行怎么断」，不动任何可见字符：CRLF 与孤立 CR 都换成 LF，
// 每行行首缩进与中间空行照旧原样保留。这样归档、xlsx、终端三处拿到的是同一份已经断好行的
// 文本，呈现层不必再各清各的。
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

// runWithTimeoutAndCancel 执行超时（context.WithTimeout + defer cancel）与外部中断。
// 后两个返回值把"到点收场 / 外部中断"报给调用方写进事实字段（TimedOut / Canceled）——
// 这两个事实不该再靠辨认报错文案反推。
func (c *Client) runWithTimeoutAndCancel(parent context.Context, session *ssh.Session, command string) (err error, timedOut bool, canceled bool) {
	ctx, cancel := context.WithTimeout(parent, c.cfg.ExecTimeout)
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- session.Run(command) }()

	select {
	case err = <-done:
		return err, false, false
	case <-ctx.Done():
		_ = session.Close()
		if parent.Err() != nil {
			return parent.Err(), false, true // 外部取消（中断 / 上级超时）
		}
		return errors.New(Message(EnvRun, runTimeout,
			fmt.Sprintf("exceeded %v", c.cfg.ExecTimeout))), true, false
	}
}

// extractExitCode 仅 *ssh.ExitError（命令以非 0 退出）返回真实退出码；
// 超时 / 中断等非命令执行错误返回 nil（GLOSSARY「退出码」）。
//
// *ssh.ExitMissingError 不在这里返回码——它没有码可返；但它**不能像从前那样被直接丢弃**，
// 转由 endFactsOf 翻成一句报文（ADR-0021 决定第 8 条：库已分好类的信息不许丢）。
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
// 这是"连上了、会话也开了，但对方拒绝执行"那一类：命令根本没跑，因此没有退出码，
// 也不能靠库的提示原文去认（措辞一变就静默失效）。
func isCommandRejected(err error) bool {
	var openErr *ssh.OpenChannelError
	return errors.As(err, &openErr)
}

// endFacts 一次执行的结束信号 → 事实字段。三条通道（命令 / 代填 / 改密）收场时登记的
// 是同一件事，这里做单点映射：
//
//   - 干净收场（err==nil）  → 退出码 0
//   - 非超时非中断的异常     → 拿得到退出码就用它，拿不到就把远端原文记为报错
//   - 超时 / 中断            → 记原文，两个标志置真
//
// 顺序要紧：超时 / 中断同样在 err 上带一个错误值，而那个值只是「命令执行超时(Ns)」——
// 先走非超时分支就会把远端说的原因盖掉（2026-09-29 实测踩到）。故由两个标志把关。
//
// 不管 SessionBegun：命令通道由本节信号推、交互与改密通道由「会话建起来没」给，
// 语义本就不一样，各调用点自行落。
type endFacts struct {
	exitCode *int
	errText  *string
	timedOut bool
	canceled bool
}

func endFactsOf(err error, timedOut, canceled bool) endFacts {
	f := endFacts{timedOut: timedOut, canceled: canceled}
	switch {
	case err == nil:
		code := 0
		f.exitCode = &code
	case !timedOut && !canceled:
		if code := extractExitCode(err); code != nil {
			f.exitCode = code
		} else if isExitMissing(err) {
			// 库已经告诉我们「会话结束了、没收到退出状态」——这本身就是分类依据，
			// 翻成报文，不再退化成靠关键词匹配库提示原文（库改措辞就静默失效）。
			f.errText = strPtr(Message(EnvSession, sessionExitMissing, err.Error()))
		} else if isCommandRejected(err) {
			// 库已经告诉我们「目标机拒了这次请求」：命令没跑，没有退出码可给。
			f.errText = strPtr(Message(EnvRun, runRejected, err.Error()))
		} else {
			f.errText = strPtr(err.Error())
		}
	default:
		f.errText = strPtr(err.Error())
	}
	return f
}

func strPtr(s string) *string { return &s }
