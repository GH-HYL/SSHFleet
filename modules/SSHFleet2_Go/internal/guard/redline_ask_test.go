package guard

import (
	"testing"

	"sshfleet2/internal/ask"
	"sshfleet2/internal/message"
)

// 红线：ask 的那一件事（摆一屏、收一行）。只判形状——非交互时交回「按缺省放行」，
// 读到输入结束交回「没答完」。问句措辞不归这里判。

func TestRedlineAskNonInteractive(t *testing.T) {
	// 话造不出来（`message.Line` 字段不导出），只能向 message 要一句——红线测试也不例外。
	// 样本取确认问句：它不随哪个阶段到期而消失（ADR-0039）。
	got := ask.Ask(message.ConfirmPrompt(), true, false)
	if got != ask.Yes {
		t.Errorf("--yes 时应交回按缺省放行，got %v", got)
	}
}
