package execute

// 02 号工单：命令模式从入口真跑完一趟。
//
// 这一条钉的是「默认套件里第一次真的连上、真的执行」——连接、认证、开会话、下发行、
// 收口这一整条路，任何一环断了它都红。面板在非终端下不渲染（见规格 Implementation
// Decisions 第七条），所以这里读的是纸与终端输出档案。

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sshfleet2/internal/contract"
)

func TestRunCommandOverFakeTarget(t *testing.T) {
	ft := startFakeTarget(t)
	host, port, err := net.SplitHostPort(ft.addr)
	if err != nil {
		t.Fatalf("拆地址失败：%v", err)
	}

	p := &contract.Plan{
		Mode:  contract.ModeCommand,
		Task:  contract.Task{Command: &contract.CommandTask{Text: "echo hi"}},
		Nodes: []contract.Node{{IP: host, Port: port, User: "tester", Password: "secret"}},
		Run:   contract.RunParams{Downlink: "echo hi", TimeoutExecute: 30, Interpreter: "bash"},
	}

	dir := t.TempDir()
	termPath := filepath.Join(dir, "terminal-output.txt")
	paper, fail := Run(context.Background(), p, contract.Landings{
		Terminal: termPath,
		ExecLog:  filepath.Join(dir, "SSHFleetExec.log"),
	}, nil)
	if fail != nil {
		t.Fatalf("不该起不来：%+v", fail)
	}

	if paper.Total != 1 || len(paper.Items) != 1 {
		t.Fatalf("收下一台，却交回 %d 台 / %d 条事实", paper.Total, len(paper.Items))
	}
	it := paper.Items[0]
	if !it.Connected {
		t.Fatalf("这一台该连上了，实际没连上：%q", it.ErrorText)
	}
	if it.ExitCode == nil || *it.ExitCode != 0 {
		t.Fatalf("退出码该是 0，实际 %v（错误原文 %q）", it.ExitCode, it.ErrorText)
	}
	if !strings.Contains(it.Output, "hi") {
		t.Fatalf("输出里该有这条命令的结果，实际 %q", it.Output)
	}

	// 终端输出档案里该有这一台的 result 行——纸上与落盘是同一件事。
	body, err := os.ReadFile(termPath)
	if err != nil {
		t.Fatalf("读终端输出档案失败：%v", err)
	}
	if !strings.Contains(string(body), host) {
		t.Fatalf("终端输出档案里没有这一台（%s）：%q", host, string(body))
	}
}
