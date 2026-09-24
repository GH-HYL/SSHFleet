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

	quiet := NewInteractor(true)
	quiet.Out = &buf
	quiet.Notice("提示：源里的软链接被过滤\n")
	if buf.Len() != 0 {
		t.Fatalf("非交互模式下提示不该输出，实际：%q", buf.String())
	}

	interactive := NewInteractor(false)
	interactive.Out = &buf
	interactive.Notice("提示：源里的软链接被过滤\n")
	if !strings.Contains(buf.String(), "软链接被过滤") {
		t.Fatalf("交互模式下提示应照常输出，实际：%q", buf.String())
	}
}
