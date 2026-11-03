// Package ask 是横切叶子包：摆出一屏问题、收一行回答。
//
// 它只收一个「要给人看的东西」，回答也只报事实（答是／答否／没答完），不判断答案的含义——
// 放不放行是闸门的结论。它不写日志：留痕由闸门拿「按缺省放行」这个事实去写。
//
// 判据见 ADR-0032 第四条。
package ask

import (
	"bufio"
	"os"
	"strings"

	"sshfleet2/internal/console"
	"sshfleet2/internal/message"
)

// Answer 是一次提问的结果。
type Answer int

const (
	// Yes 答了是（或非交互下按缺省放行）。
	Yes Answer = iota
	// No 答了否，或缺省是「否」而使用者只敲了回车。
	No
	// NoInput 读到输入结束——没答完。
	NoInput
)

// Ask 打出一屏、收一行回答。
//
// assumeYes（--yes）时不打屏、不读输入，直接交回「按缺省放行」这个事实；留痕由调用方写。
func Ask(screen message.Line, assumeYes bool, defaultYes bool) Answer {
	// 屏照打：--yes 只替人回答了「y」，不该连「命中了危险规则」这件事也抹掉。
	console.Print(screen)
	if assumeYes {
		return Yes
	}
	return readAnswer(message.ConfirmPrompt(), defaultYes)
}

// stdinReader 整个进程共用一条。
//
// 不能每次提问新建一个 `bufio.Scanner`：它会把后续输入提前读进自己的缓冲，用完随它一起
// 丢掉——一次交互里问两三句（回退确认 → 并发建议 → 确认屏）时，第二句就永远读不到，
// 屏幕上表现为「输入结束，操作已取消」。老版共用一条 reader，这里对齐。
var stdinReader = bufio.NewReaderSize(os.Stdin, 64*1024)

// readLine 读一行；流结束时 ok 为假。最后一行没有换行符也算读到了。
func readLine() (string, bool) {
	line, err := stdinReader.ReadString('\n')
	if err != nil && line == "" {
		return "", false
	}
	return strings.TrimRight(line, "\r\n"), true
}

// Show 只打一屏、不问。用在「先说一段、后面还有别的屏」的场合：
// 确认屏之后紧跟上传内容树与并发建议，中间没有要收的回答。
func Show(screen message.Line) {
	console.Print(screen)
}

// AskWith 与 Ask 同，但**屏与问句都由调用方给**。
//
// 危险提醒那句问句与确认屏那句不是一句话；屏幕也一样——确认屏是一整张表。
// [Y/n] 与 [y/N] 的形态仍由这里按 defaultYes 定：使用者看方括号就知道回车会怎样。
//
// 答了 y 就紧跟着打那两行「开始执行」：老版收下 y 之后在确认函数里当场打（confirm.go:112），
// 它是这次交互的收尾，不是开工那一侧的话——所以住 ask，随这一屏一起出去。危险提醒档走
// `Ask`，没有这一笔。
func AskWith(screen, question message.Line, defaultYes bool) Answer {
	answer := AskQuestion(screen, question, defaultYes)
	if answer == Yes {
		console.Print(message.StartLine())
	}
	return answer
}

// AskQuestion 与 AskWith 同，只是答是之后**不打**那两行「开始执行」。
//
// 那两行属于确认屏那一问（它是这次交互的收尾）；回退确认这类问句答完还有参数屏要摆，
// 在这儿打就等于提前宣布开工。
func AskQuestion(screen, question message.Line, defaultYes bool) Answer {
	console.Print(screen)
	return readAnswer(question, defaultYes)
}

// readAnswer 打问句与方括号提示，再收一行。
func readAnswer(question message.Line, defaultYes bool) Answer {
	console.Print(message.PromptWith(question, defaultYes))

	line, ok := readLine()
	if !ok {
		console.Print(message.InputEndLine())
		return NoInput
	}
	line = strings.ToLower(strings.TrimSpace(line))
	switch {
	case line == "" && defaultYes:
		return Yes
	case line == "" || line == "n" || line == "no":
		return No
	case line == "y" || line == "yes":
		return Yes
	default:
		return No
	}
}
