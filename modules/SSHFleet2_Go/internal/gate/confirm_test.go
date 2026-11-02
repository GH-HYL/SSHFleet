package gate

import (
	"strings"
	"testing"

	"sshfleet2/internal/contract"
)

// 信封：标签说是哪一种活，载荷就得是那一份、而且只有那一份。
//
// 这一组只在纸被造错时才响，正常使用撞不上——所以判的是「拦不拦」，不是措辞。
func TestEnvelopeRefusesMismatch(t *testing.T) {
	cases := []struct {
		name string
		make func() *contract.Plan
	}{
		{"标签是命令，载荷是空的", func() *contract.Plan {
			return &contract.Plan{Mode: contract.ModeCommand, Run: contract.RunParams{Yes: true}}
		}},
		{"标签是命令，装的却是脚本", func() *contract.Plan {
			return &contract.Plan{Mode: contract.ModeCommand, Task: contract.Task{Script: &contract.ScriptTask{Path: "a.sh"}}, Run: contract.RunParams{Yes: true}}
		}},
		{"同时装了两份载荷", func() *contract.Plan {
			return &contract.Plan{Mode: contract.ModeCommand, Task: contract.Task{
				Command: &contract.CommandTask{Text: "uptime"},
				Script:  &contract.ScriptTask{Path: "a.sh"},
			}, Run: contract.RunParams{Yes: true}}
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r, _ := Run(c.make())
			if r == nil {
				t.Fatal("对不上的信封应当被拒")
			}
			if !strings.Contains(r.Say.Text(), "这次要做什么与它的载荷对不上") {
				t.Fatalf("话不对：%s", r.Say.Text())
			}
		})
	}
}

// 合法的信封不该被这一条拦（其它判据可能拦，但那一句不该出现）。
func TestEnvelopePassesWellFormed(t *testing.T) {
	p := &contract.Plan{
		Mode: contract.ModeCommand,
		Task: contract.Task{Command: &contract.CommandTask{Text: "uptime"}},
		Run:  contract.RunParams{Yes: true},
	}
	if r, _ := Run(p); r != nil && strings.Contains(r.Say.Text(), "载荷对不上") {
		t.Fatalf("合法的纸不该被信封那条拦：%s", r.Say.Text())
	}
}

// 非交互（--yes）：不问、不拒，但要把「跳过确认」这件事交上去留痕。
//
// 屏幕上那行 `[非交互模式] 跳过…` 由 ask 在真进程里打（A/B 与老版逐字比过），
// 这一条只判闸门交回的结论与那句留痕。
func TestConfirmSkippedWhenYes(t *testing.T) {
	p := &contract.Plan{
		Mode: contract.ModeCommand,
		Task: contract.Task{Command: &contract.CommandTask{Text: "uptime"}},
		Run:  contract.RunParams{Yes: true},
	}
	r, trace := Run(p)
	if r != nil {
		t.Fatalf("--yes 不该拒：%s", r.Say.Text())
	}
	if trace.Text() != "非交互模式（--yes）跳过执行选项/参数确认，直接执行" {
		t.Fatalf("留痕那句话不对：%q", trace.Text())
	}
}

// 前面任何一条判据拦下来，就不该走到确认那一步（否则屏幕上会先摊一张表、再说不行）。
func TestConfirmNotReachedWhenEarlierCheckRefuses(t *testing.T) {
	p := &contract.Plan{
		Mode: contract.ModeCommand,
		Task: contract.Task{Command: &contract.CommandTask{Text: "   "}}, // -c 只有空白
		Run:  contract.RunParams{Yes: true},
	}
	r, _ := Run(p)
	if r == nil {
		t.Fatal("空命令应当被拒")
	}
	if strings.Contains(r.Say.Text(), "以上确认无误") {
		t.Fatal("被前一条拦下时不该出确认屏")
	}
}
