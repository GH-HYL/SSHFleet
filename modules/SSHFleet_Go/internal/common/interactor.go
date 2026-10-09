package common

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/term"

	"sshfleet/internal/log"
)

// ErrCancelled 用户取消交互；main 据此以 1 退出且不附加 [ERROR] 前缀
// （取消文案由交互器自己打印，对位旧 interaction 模块的行为）。
var ErrCancelled = errors.New("用户取消输入")

// 取消提示与 yes/no 掐示的配色（对位旧 constants.py / interaction.py：提示红、取消黄）。
const (
	colorReset  = "\x1b[0m"
	colorRed    = "\x1b[31m"
	colorYellow = "\x1b[33m"
)

// Interactor 是全工具唯一的用户交互入口（需注入依赖的类型，spec D32 条件 4）。
// 归属 internal/common：nodelist（字段补全交互）与 confirm（参数确认）两个功能目录
// 共用，且自身不持有状态——In/Out 为注入依赖。
type Interactor struct {
	In             io.Reader
	Out            io.Writer
	Disinteractive bool        // 对位 --disinteractive：Confirm 显式返回已确认（不再借用 yorn 值）
	Log            *log.Logger // 留痕出口：Confirm 在非交互（--yes）跳过确认时写一条

	sc *bufio.Scanner
}

func NewInteractor(disinteractive bool, logger *log.Logger) *Interactor {
	return &Interactor{In: os.Stdin, Out: os.Stdout, Disinteractive: disinteractive, Log: logger}
}

// Notice 非交互性提示（校验重试提示、清单清洗说明等）：只往注入的输出流写，
// 不读输入。存在这条通道，是为了让「提示」与「提问」都走同一个出口——
// 提示直接写 os.Stdout 会让调用方无法在测试里捕获它们。
// Notice 输出一条运行期提示（非报错、不阻塞执行的那种）。
//
// **非交互模式（--yes）下静默**：它就是 L61 所说的「静默总闸门」——既然是"跳过所有
// 确认直接执行"，教学类提示就不该再来刷屏。注意这只关掉终端输出，报错（阻塞类）
// 走的是 fatal，不受影响。
func (i *Interactor) Notice(text string) {
	if i.Disinteractive {
		return
	}
	fmt.Fprint(i.Out, text)
}

// Prompt 读取一行文本。EOF / 读取出错：打印取消文案并返回 ErrCancelled。
//
// **当前没有调用者**：补输入交互已按 L16 从用户可见面整体移除（缺字段一律报错），
// 交互只剩 Confirm 一条路。这里按 L19 的退役手法保留实现——终端无回显读取这类细节
// 重写成本不低，留着供日后改回来。
func (i *Interactor) Prompt(prompt string) (string, error) {
	fmt.Fprint(i.Out, prompt)
	line, ok := i.readLine()
	if !ok {
		fmt.Fprint(i.Out, "\n"+colorYellow+"用户取消输入"+colorReset+"\n")
		return "", ErrCancelled
	}
	return line, nil
}

// PromptPassword 读取敏感输入，终端上不回显（对位 getpass）；
// 输入不是终端时降级为普通读取（对位 getpass 的 GetPassWarning 降级）。
// 保留理由同 Prompt：无调用者，属退役能力。
func (i *Interactor) PromptPassword(prompt string) (string, error) {
	if term.IsTerminal(int(os.Stdin.Fd())) {
		fmt.Fprint(i.Out, prompt)
		raw, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Fprint(i.Out, "\n")
		if err != nil {
			fmt.Fprint(i.Out, colorYellow+"用户取消输入"+colorReset+"\n")
			return "", ErrCancelled
		}
		return string(raw), nil
	}
	return i.Prompt(prompt)
}

// ConfirmReq 一次 yes/no 确认的全部入参（打包传参，避免位置参数一路加长）。
type ConfirmReq struct {
	Prompt     string // 交互提问语；[Y/n] / [y/N] 后缀由 Confirm 自己拼
	DefaultYes bool   // 回车缺省
	Trace      string // 留痕文案：非交互（--yes）跳过确认时写进工具日志；空 = 不留痕
	SkipLine   string // 非交互跳过时上屏的一行（自带换行）；空 = 只留痕、不上屏
}

// Confirm 全工具唯一的 yes/no 入口。提问语、回车缺省、留痕文案都走参数，
// 非交互（--yes）判定收在函数内——不再让每个调用方各写一套「跳过时该干什么」，
// 后面再加确认项也不会漏掉留痕。
//
// 三个出口：
//   - 非交互（--yes）：不打屏、不读输入，按 Trace 留痕（SkipLine 非空则再上屏一行），视为已确认；
//   - 读到 EOF：打印「输入结束，操作已取消」并返回 ErrCancelled（Ctrl+C 由 main 级信号处理接管）；
//   - 正常：回车取 DefaultYes，答否返回 false，由调用方决定怎么处置。
func (i *Interactor) Confirm(req ConfirmReq) (bool, error) {
	if i.Disinteractive {
		i.trace(req.Trace)
		if req.SkipLine != "" {
			fmt.Fprint(i.Out, req.SkipLine)
		}
		return true, nil
	}
	hint := colorRed + "[y/N]" + colorReset
	if req.DefaultYes {
		hint = colorRed + "[Y/n]" + colorReset
	}
	fmt.Fprintf(i.Out, "%s %s: ", req.Prompt, hint)
	line, ok := i.readLine()
	if !ok {
		fmt.Fprint(i.Out, "\n"+colorYellow+"输入结束，操作已取消"+colorReset+"\n")
		return false, ErrCancelled
	}
	line = strings.ToLower(strings.TrimSpace(line))
	if line == "" {
		return req.DefaultYes, nil
	}
	return line == "y" || line == "yes", nil
}

// trace 往工具日志写一条留痕；无日志口（测试）或文案为空时静默。
func (i *Interactor) trace(text string) {
	if text != "" && i.Log != nil {
		i.Log.Warn(text)
	}
}

// readLine 从注入的输入流读一行；流结束时返回 ok=false。
func (i *Interactor) readLine() (string, bool) {
	if i.sc == nil {
		i.sc = bufio.NewScanner(i.In)
		i.sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	}
	if !i.sc.Scan() {
		return "", false
	}
	return i.sc.Text(), true
}
