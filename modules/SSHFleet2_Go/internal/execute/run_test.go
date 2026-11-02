package execute

// 执行站入口：收得到纸、连不上时那一格有人填、按纸上顺序交回、取消时不拖着跑。

import (
	"context"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	"sshfleet2/internal/contract"
)

// TestRunRecordsConnectFailure 一台连不上：造一条事实，报文进「错误原文」。
func TestRunRecordsConnectFailure(t *testing.T) {
	paper, fail := Run(context.Background(), plan("127.0.0.1", deadPort(t)))
	if fail != nil {
		t.Fatalf("不该起不来：%+v", fail)
	}
	if paper.Total != 1 {
		t.Fatalf("总台数应为 1，实际 %d", paper.Total)
	}
	if len(paper.Items) != 1 {
		t.Fatalf("应造一条事实，实际 %d 条", len(paper.Items))
	}
	item := paper.Items[0]
	if item.Connected {
		t.Fatal("端口没人听，却记成连上了")
	}
	if !strings.HasPrefix(item.ErrorText, "sshfleet.connect.refused: ") {
		t.Fatalf("错误原文应是「拒绝网络连接」那条报文，实际 %q", item.ErrorText)
	}
	if item.ConnectCost <= 0 {
		t.Fatalf("连接耗时没记：%v", item.ConnectCost)
	}
}

// TestRunKeepsNodeOrder 并发跑完，交回的顺序仍是纸上那个顺序（不按完成先后）。
func TestRunKeepsNodeOrder(t *testing.T) {
	ports := []string{deadPort(t), deadPort(t), deadPort(t)}
	p := &contract.Plan{
		Mode: contract.ModeCommand,
		Task: contract.Task{Command: &contract.CommandTask{Text: "uptime"}},
		Nodes: []contract.Node{
			{IP: "127.0.0.2", Port: ports[0], User: "u", Password: "p"},
			{IP: "127.0.0.1", Port: ports[1], User: "u", Password: "p"},
			{IP: "127.0.0.3", Port: ports[2], User: "u", Password: "p"},
		},
	}
	paper, fail := Run(context.Background(), p)
	if fail != nil {
		t.Fatalf("不该起不来：%+v", fail)
	}
	if len(paper.Items) != 3 {
		t.Fatalf("应造三条事实，实际 %d 条", len(paper.Items))
	}
	want := []string{"127.0.0.2", "127.0.0.1", "127.0.0.3"}
	for i, ip := range want {
		if paper.Items[i].IP != ip {
			t.Fatalf("第 %d 条应是 %s，实际 %s", i+1, ip, paper.Items[i].IP)
		}
	}
}

// TestRunCanceledContextStopsAtOnce 已取消的 context：立刻收场，一条「连上了」都不许有。
//
// 开了几个 worker 是内部事，所以不断言「一条事实都没有」——只断言这次不拖、并且造出来的
// 那些行都带着「被取消」这个事实。后台 goroutine 泄漏由收场时间与 `-race` 一起兜。
func TestRunCanceledContextStopsAtOnce(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	p := plan("127.0.0.1", deadPort(t))
	done := make(chan contract.ResultPaper, 1)
	go func() {
		paper, _ := Run(ctx, p)
		done <- paper
	}()

	select {
	case paper := <-done:
		for _, item := range paper.Items {
			if item.Connected {
				t.Fatalf("取消之后还记成连上了：%+v", item)
			}
			if !item.Cancelled {
				t.Fatalf("这一台是被取消收场的，却没记那一格：%+v", item)
			}
		}
	case <-time.After(5 * time.Second):
		t.Fatal("已取消的 context 下，建连没有立刻收场")
	}
}

// TestRunWithoutNodes 没节点：交回零条的纸，不是「起不来」。
func TestRunWithoutNodes(t *testing.T) {
	paper, fail := Run(context.Background(), &contract.Plan{Mode: contract.ModeCommand})
	if fail != nil {
		t.Fatalf("不该起不来：%+v", fail)
	}
	if paper.Total != 0 || len(paper.Items) != 0 {
		t.Fatalf("没节点时应该是零条，实际总台数 %d、事实 %d 条", paper.Total, len(paper.Items))
	}
	if paper.StoppedAt.Before(paper.StartedAt) {
		t.Fatalf("收工时间早于开工时间：%v → %v", paper.StartedAt, paper.StoppedAt)
	}
}

// plan 造一张只有一个节点的命令模式纸。
func plan(ip, port string) *contract.Plan {
	return &contract.Plan{
		Mode:  contract.ModeCommand,
		Task:  contract.Task{Command: &contract.CommandTask{Text: "uptime"}},
		Nodes: []contract.Node{{IP: ip, Port: port, User: "u", Password: "p"}},
		Run:   contract.RunParams{TimeoutExecute: 30, Interpreter: "bash"},
	}
}

// deadPort 要一个**此刻没人听**的本机端口：先占住、拿到号、再让出来。
func deadPort(t *testing.T) string {
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
