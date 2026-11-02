package guard

import (
	"testing"

	"sshfleet2/internal/contract"
	"sshfleet2/internal/gate"
)

// 红线：闸门的入口（递一张纸、收一张纸 + 一个拒绝）。
//
// 只判形状：不让走时拒绝非空、放行时为 nil。措辞对不对不归这里判。
// 纸上那几格是造的——04 才有真纸，这里只需一个「结构上说得通」的纸（造数据只住测试）。

func redlinePaper() *contract.Plan {
	return &contract.Plan{
		Mode: contract.ModeCommand,
		Task: contract.Task{Command: &contract.CommandTask{Text: "uptime"}},
		// Yes 为真：这张纸只用来判形状，不该真的去等一行输入。
		Run: contract.RunParams{Concurrency: 0, TimeoutConnect: 10, TimeoutExecute: 60, TimeoutTransfer: 300, Yes: true},
	}
}

func TestRedlineGateRefusesBadCombination(t *testing.T) {
	bad := redlinePaper()
	bad.Task.Command = nil
	bad.Problems = []contract.Problem{{Kind: contract.ProblemMultiMode, Detail: "--command、--download"}}

	refusal, _ := gate.Run(bad)
	if refusal == nil {
		t.Fatalf("给了多个模式应当返回一个拒绝")
	}
	if refusal.Say.S == "" {
		t.Errorf("拒绝要带一屏话")
	}
}

func TestRedlineGatePassesCleanPaper(t *testing.T) {
	refusal, _ := gate.Run(redlinePaper())
	if refusal != nil {
		t.Fatalf("一张干净的纸不该被拒：%s", refusal.Say.S)
	}
}
