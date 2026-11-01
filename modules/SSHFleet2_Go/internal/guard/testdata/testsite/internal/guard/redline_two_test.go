package guard

import (
	"testing"

	"fixture/internal/gate"
	"fixture/internal/parse"
)

// TestTwoPins 钉了两个包——类六的坏样本。
func TestTwoPins(t *testing.T) {
	_ = gate.Refusal{}
	_ = parse.Run
}
