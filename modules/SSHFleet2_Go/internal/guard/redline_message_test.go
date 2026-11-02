package guard

import (
	"strings"
	"testing"

	"sshfleet2/internal/message"
)

// 红线：拼句包的那一件事——事实进去，一屏文本出来。只判形状（有产出、版本行在
// 首行下方），一个字的措辞都不判——措辞归老版对照与 A/B 比对。

func TestRedlineMessageRendersHelp(t *testing.T) {
	entries := []message.HelpEntry{
		{Kind: message.EntryOption, Short: "-x", Long: "--example", Desc: "说明"},
	}
	say := message.HelpScreen("PROG", "vTEST", 0, entries)
	if say.Text() == "" {
		t.Fatalf("帮助屏不应是空的")
	}
	lines := strings.Split(say.Text(), "\n")
	if len(lines) < 2 || !strings.HasPrefix(lines[1], "版本: vTEST") {
		t.Errorf("首行下一行应以「版本: v」开头：\n%s", say.Text())
	}
}
