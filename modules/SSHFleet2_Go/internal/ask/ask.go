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

// readLine 从标准输入读一行；流结束时 ok 为假。
func readLine() (string, bool) {
	sc := bufio.NewScanner(os.Stdin)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	if !sc.Scan() {
		return "", false
	}
	return sc.Text(), true
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
	console.Print(screen)
	answer := readAnswer(question, defaultYes)
	if answer == Yes {
		console.Print(message.StartLine())
	}
	return answer
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
