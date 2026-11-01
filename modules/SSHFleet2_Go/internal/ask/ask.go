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
	"sshfleet2/internal/contract"
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
func Ask(screen contract.Text, assumeYes bool, defaultYes bool) Answer {
	if assumeYes {
		return Yes
	}
	console.Print(screen)

	line, ok := readLine()
	if !ok {
		console.Print(message.InputEndLine())
		return NoInput
	}
	line = strings.ToLower(strings.TrimSpace(line))
	if line == "" {
		if defaultYes {
			return Yes
		}
		return No
	}
	if line == "y" || line == "yes" {
		return Yes
	}
	return No
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
