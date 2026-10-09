// 交互器的行为约定：它只管"确认"与"提示"两件事，不承担任何补输入。
package common

import (
	"bytes"
	"strings"
	"testing"
)

// 非交互模式（--yes）是提示的静默总闸门（L61）：既然是"跳过所有确认直接执行"，
// 教学类提示就不该再来刷屏。交互模式下照常输出。
func TestNoticeSilentInNonInteractiveMode(t *testing.T) {
	var buf bytes.Buffer

	quiet := NewInteractor(true, nil)
	quiet.Out = &buf
	quiet.Notice("提示：源里的软链接被过滤\n")
	if buf.Len() != 0 {
		t.Fatalf("非交互模式下提示不该输出，实际：%q", buf.String())
	}

	interactive := NewInteractor(false, nil)
	interactive.Out = &buf
	interactive.Notice("提示：源里的软链接被过滤\n")
	if !strings.Contains(buf.String(), "软链接被过滤") {
		t.Fatalf("交互模式下提示应照常输出，实际：%q", buf.String())
	}
}

// Confirm：回车取缺省、答 n 为否；非交互（--yes）视为已确认且只上屏 SkipLine、不提问。
func TestConfirmDefaultsAndNonInteractive(t *testing.T) {
	// 每个用例各起一个交互器：readLine 的 scanner 只建一次，复用会读到已耗尽的旧流
	var discard bytes.Buffer
	inYes := NewInteractor(false, nil)
	inYes.Out = &discard
	inYes.In = strings.NewReader("\n")
	if got, err := inYes.Confirm(ConfirmReq{Prompt: "继续", DefaultYes: true}); err != nil || !got {
		t.Fatalf("回车应取缺省 true，实际 got=%v err=%v", got, err)
	}

	inNo := NewInteractor(false, nil)
	inNo.Out = &discard
	inNo.In = strings.NewReader("n\n")
	if got, err := inNo.Confirm(ConfirmReq{Prompt: "继续", DefaultYes: true}); err != nil || got {
		t.Fatalf("答 n 应为 false，实际 got=%v err=%v", got, err)
	}

	// --yes：不读输入也视为已确认；除 SkipLine 外不上屏
	quiet := NewInteractor(true, nil)
	var qout bytes.Buffer
	quiet.Out = &qout
	quiet.In = strings.NewReader("") // 无输入可读
	got, err := quiet.Confirm(ConfirmReq{Prompt: "继续", DefaultYes: false, SkipLine: "跳过\n"})
	if err != nil || !got {
		t.Fatalf("--yes 应视为已确认，实际 got=%v err=%v", got, err)
	}
	if qout.String() != "跳过\n" {
		t.Fatalf("--yes 下应只上屏 SkipLine，实际 %q", qout.String())
	}
}
