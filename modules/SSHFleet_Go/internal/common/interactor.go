package common

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/term"
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
	Disinteractive bool // 对位 --disinteractive：Confirm 显式返回已确认（不再借用 yorn 值）

	sc *bufio.Scanner
}

func NewInteractor(disinteractive bool) *Interactor {
	return &Interactor{In: os.Stdin, Out: os.Stdout, Disinteractive: disinteractive}
}

// Notice 非交互性提示（校验重试提示、清单清洗说明等）：只往注入的输出流写，
// 不读输入。存在这条通道，是为了让「提示」与「提问」都走同一个出口——
// 提示直接写 os.Stdout 会让调用方无法在测试里捕获它们。
func (i *Interactor) Notice(text string) {
	fmt.Fprint(i.Out, text)
}

// Prompt 读取一行文本。EOF / 读取出错：打印取消文案并返回 ErrCancelled。
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

// Confirm yes/no 确认。defaultYes 决定回车缺省与 [Y/n]/[y/N] 措辞。
// 非交互模式显式返回已确认（spec 实现层差异：不再借用 yorn 参数值）。
// EOF：打印「输入结束，操作已取消」并返回 ErrCancelled；
// Ctrl+C 由 main 级信号处理接管（M3 细化中断语义）。
func (i *Interactor) Confirm(prompt string, defaultYes bool) (bool, error) {
	if i.Disinteractive {
		return true, nil
	}
	hint := colorRed + "[y/N]" + colorReset
	if defaultYes {
		hint = colorRed + "[Y/n]" + colorReset
	}
	fmt.Fprintf(i.Out, "%s %s: ", prompt, hint)
	line, ok := i.readLine()
	if !ok {
		fmt.Fprint(i.Out, "\n"+colorYellow+"输入结束，操作已取消"+colorReset+"\n")
		return false, ErrCancelled
	}
	line = strings.ToLower(strings.TrimSpace(line))
	if line == "" {
		return defaultYes, nil
	}
	return line == "y" || line == "yes", nil
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
