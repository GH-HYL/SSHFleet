package ask

// 一次交互里问两句：两句都要读得到。这条盯的是「每条问句各开一个 bufio.Scanner」那个
// 回退——Scanner 会把后续输入提前读进自己的缓冲、用完一起丢掉，于是第二句永远读到 EOF，
// 屏幕上表现为「输入结束，操作已取消」（18 号单 A/B 撞出来的：回退确认 → 并发建议 → 确认屏）。

import (
	"bufio"
	"strings"
	"testing"

	"sshfleet2/internal/message"
)

func TestAskReadsConsecutiveAnswers(t *testing.T) {
	old := stdinReader
	stdinReader = bufio.NewReader(strings.NewReader("y\nn\n"))
	t.Cleanup(func() { stdinReader = old })

	if got := AskQuestion(message.Line{}, message.Line{}, true); got != Yes {
		t.Fatalf("第一问该读到「是」，实际 %v", got)
	}
	if got := AskQuestion(message.Line{}, message.Line{}, true); got != No {
		t.Fatalf("第二问该读到「否」，实际 %v（读到 EOF 就是每条问句各开了一个 Scanner）", got)
	}
}
