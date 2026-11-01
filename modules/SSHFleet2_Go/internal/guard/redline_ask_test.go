package guard

import (
	"testing"

	"sshfleet2/internal/ask"
	"sshfleet2/internal/contract"
)

// 红线：ask 的那一件事（摆一屏、收一行）。只判形状——非交互时交回「按缺省放行」，
// 读到输入结束交回「没答完」。问句措辞不归这里判。

func TestRedlineAskNonInteractive(t *testing.T) {
	got := ask.Ask(contract.Text{S: "问一句"}, true, false)
	if got != ask.Yes {
		t.Errorf("--yes 时应交回按缺省放行，got %v", got)
	}
}
