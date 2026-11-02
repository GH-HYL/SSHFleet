package message

import (
	"regexp"
	"strings"
	"testing"

	"sshfleet2/internal/contract"
)

// 确认屏的版式：这一屏是使用者按下回车之前唯一看到的全貌，字与列位都照着老版录。
//
// 上色码在这里剥掉再比——颜色对不对由 A/B 与老二进制逐字节对（`build/建站04_05_2026-11-02_1015/`），
// 这一条管的是「哪几行、什么字、对齐没有」。
var ansi = regexp.MustCompile("\x1b\\[[0-9;]*m")

func stripANSI(s string) string { return ansi.ReplaceAllString(s, "") }

func confirmTestPlan() *contract.Plan {
	return &contract.Plan{
		Mode:      contract.ModeCommand,
		Task:      contract.Task{Command: &contract.CommandTask{Text: "uptime"}},
		Resources: contract.Resources{Manifest: "ok.csv"},
		Nodes:     []contract.Node{{IP: "10.0.0.1"}},
		Run:       contract.RunParams{TimeoutConnect: 10, TimeoutExecute: 60, Remark: "uptime"},
	}
}

func TestConfirmScreenLooksLikeOldTool(t *testing.T) {
	got := stripANSI(ConfirmScreen(confirmTestPlan()).S)

	want := "\n" +
		"╔════════════════════════════════════════════════════╗\n" +
		"║             SSHFleet - 执行选项/参数确认             ║\n" +
		"╚════════════════════════════════════════════════════╝\n" +
		"\n" +
		"▶ 执行模式-→   命令模式\n" +
		"▶ 命令内容-→   uptime\n" +
		"\n" +
		"▶ 节点清单-→   ok.csv\n" +
		"▶ 节点数量-→   1\n" +
		"▶ 并发数  -→   1（全部并行）\n" +
		"\n" +
		"▶ 连接超时-→   10s\n" +
		"▶ 执行超时-→   60s\n" +
		"▶ 备注    -→   uptime\n"

	if got != want {
		t.Fatalf("确认屏与老版对不上\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

// 没指定 -n 时并发数写节点数，并补一句「全部并行」——这个数从哪来必须说得出。
func TestConfirmScreenSaysAllParallelWhenUnset(t *testing.T) {
	p := confirmTestPlan()
	p.Nodes = []contract.Node{{IP: "a"}, {IP: "b"}, {IP: "c"}}

	if got := stripANSI(ConfirmScreen(p).S); !strings.Contains(got, "▶ 并发数  -→   3（全部并行）") {
		t.Fatalf("没标「全部并行」：\n%s", got)
	}
}

// 指定了 -n 就只写那个数，不加注。
func TestConfirmScreenShowsGivenConcurrency(t *testing.T) {
	p := confirmTestPlan()
	p.Run.Concurrency = 4

	got := stripANSI(ConfirmScreen(p).S)
	if !strings.Contains(got, "▶ 并发数  -→   4\n") {
		t.Fatalf("并发数不对：\n%s", got)
	}
	if strings.Contains(got, "全部并行") {
		t.Fatalf("指定了 -n 不该再标「全部并行」：\n%s", got)
	}
}

// 没有备注就没有那一行（自动生成之后一般都有，但空着的时候不能摆个空标签）。
func TestConfirmScreenOmitsEmptyRemark(t *testing.T) {
	p := confirmTestPlan()
	p.Run.Remark = ""

	if got := stripANSI(ConfirmScreen(p).S); strings.Contains(got, "备注") {
		t.Fatalf("空备注不该占一行：\n%s", got)
	}
}
