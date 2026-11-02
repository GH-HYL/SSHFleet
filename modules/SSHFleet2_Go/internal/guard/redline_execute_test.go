package guard

import (
	"context"
	"net"
	"strconv"
	"strings"
	"testing"

	"sshfleet2/internal/contract"
	"sshfleet2/internal/execute"
)

// 红线：执行站的那一件事——收得下一张纸，按台数交回事实，连接成败那一格有人填。
//
// 只钉形状与流向，不钉值：连不上是端口没人听的必然结果，这里不去断言它算成功还是失败
// （那是结算站按分类解释的事，ADR-0035 决定四）。也不用真机器：本机占一个端口再让出来，
// 拨过去立刻被拒。
func TestRedlineExecuteRecordsConnectFact(t *testing.T) {
	node := contract.Node{IP: "127.0.0.1", Port: closedPort(t), User: "u", Password: "p"}
	paper, fail := execute.Run(context.Background(), &contract.Plan{
		Mode:  contract.ModeCommand,
		Task:  contract.Task{Command: &contract.CommandTask{Text: "uptime"}},
		Nodes: []contract.Node{node},
	})
	if fail != nil {
		t.Fatalf("不该起不来：%+v", fail)
	}
	if paper.Total != 1 || len(paper.Items) != 1 {
		t.Fatalf("收下一张纸，却没按台数交回事实：总台数 %d，事实 %d 条", paper.Total, len(paper.Items))
	}
	if !strings.HasPrefix(paper.Items[0].ErrorText, "sshfleet.connect.") {
		t.Fatalf("连接成败那一格没人填：%q", paper.Items[0].ErrorText)
	}
}

// closedPort 要一个此刻没人听的本机端口：先占住、拿到号、再让出来。
func closedPort(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("占端口失败：%v", err)
	}
	addr, ok := l.Addr().(*net.TCPAddr)
	if !ok {
		_ = l.Close()
		t.Fatalf("拿到的不是 TCP 地址：%v", l.Addr())
	}
	port := addr.Port
	if err := l.Close(); err != nil {
		t.Fatalf("让出端口失败：%v", err)
	}
	return strconv.Itoa(port)
}
