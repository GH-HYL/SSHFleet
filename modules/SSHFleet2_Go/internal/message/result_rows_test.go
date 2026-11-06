package message

import (
	"testing"
	"time"

	"sshfleet2/internal/contract"
)

// 表格行：连接失败那台照样有「执行」行（老版表格无条件印，终端文本不印），
// 错误只有失败那台有，输出逐行，末尾一条分隔行。
func TestResultRowsConnectFailure(t *testing.T) {
	res := contract.NodeResult{
		IP:        "10.0.0.1",
		Connected: false,
		Category:  "连接被拒",
		ErrorText: "sshfleet.connect.refused: dial tcp: connection refused",
	}
	got := ResultRows(res, contract.ModeCommand)

	want := []ResultRow{
		{Event: "连接: 失败 - 0.000s"},
		{Event: "执行: 失败 - 0.000s"},
		{Event: RowEventError, Detail: "sshfleet.connect.refused: dial tcp: connection refused"},
		{Event: "分类: 连接被拒"},
		{Separator: true},
	}
	assertRows(t, got, want)
}

// 成功那台：没有错误行；输出连着几个空行也照样各占一行。
func TestResultRowsSuccessWithMultilineOutput(t *testing.T) {
	res := contract.NodeResult{
		IP:          "10.0.0.2",
		Connected:   true,
		ConnectCost: 123 * time.Millisecond,
		Cost:        456 * time.Millisecond,
		Category:    "执行成功",
		Output:      "第一行\n\n第三行",
	}
	got := ResultRows(res, contract.ModeCommand)

	want := []ResultRow{
		{Event: "连接: 成功 - 0.123s"},
		{Event: "执行: 成功 - 0.456s"},
		{Event: RowEventOutput, Detail: "第一行", Output: true},
		{Event: RowEventOutput, Detail: "", Output: true},
		{Event: RowEventOutput, Detail: "第三行", Output: true},
		{Event: "分类: 执行成功"},
		{Separator: true},
	}
	assertRows(t, got, want)
}

// 连不上又没有原文时不留一行空白的「错误」——那是「有输出但看不见」的错觉。
func TestResultRowsNoEmptyErrorRow(t *testing.T) {
	got := ResultRows(contract.NodeResult{IP: "10.0.0.3", Category: "连接超时"}, contract.ModeCommand)
	for _, r := range got {
		if r.Event == RowEventError {
			t.Fatalf("原文空着不该造错误行：%+v", got)
		}
	}
}

func assertRows(t *testing.T, got, want []ResultRow) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("行数不对：got %d want %d\n%+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("第 %d 行不对：\n got=%+v\nwant=%+v", i+1, got[i], want[i])
		}
	}
}
