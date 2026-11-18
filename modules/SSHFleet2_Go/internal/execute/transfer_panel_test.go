package execute

// 面板上的「完成台数」由收口那一步回填（`closeNode` → 聚合器 → 传输面板的快照）。
// 非终端下面板压根不画，默认套件里把那一行调用删掉也不会红；所以这里递一个流进 `Run`
// （ADR-0061），让面板真的画一遍，读它最后一帧。

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sshfleet2/internal/contract"
	"sshfleet2/internal/message"
)

func TestRunTransferPanelCountsCompletedNodes(t *testing.T) {
	ft := startFakeTarget(t)
	node := fakeNode(t, ft)

	const body = "gamma\n"
	localDir := t.TempDir()
	localFile := filepath.Join(localDir, "a.txt")
	if err := os.WriteFile(localFile, []byte(body), 0o644); err != nil {
		t.Fatalf("写本地文件失败：%v", err)
	}
	remoteDir := ft.path("panel")
	if err := os.MkdirAll(ft.local(remoteDir), 0o755); err != nil {
		t.Fatalf("建目标机上的落点目录失败：%v", err)
	}

	p := &contract.Plan{
		Mode: contract.ModeUpload,
		Task: contract.Task{
			Path: remoteDir,
			Upload: &contract.UploadTask{
				Source: localDir,
				Files:  []contract.UploadFile{{Path: localFile, Rel: "a.txt", Size: int64(len(body))}},
			},
		},
		Nodes: []contract.Node{node},
		Run:   contract.RunParams{TimeoutExecute: 30},
	}

	var panel bytes.Buffer
	dir := t.TempDir()
	paper, fail := Run(context.Background(), p, contract.Landings{
		Terminal: filepath.Join(dir, "terminal-output.txt"),
		ExecLog:  filepath.Join(dir, "SSHFleetExec.log"),
	}, &panel)
	if fail != nil {
		t.Fatalf("不该起不来：%+v", fail)
	}
	if len(paper.Items) != 1 {
		t.Fatalf("该交回一台的事实，实际 %d 条", len(paper.Items))
	}

	// 两块面板写同一个流：命令那一行是「已完成 1/1」，传输面板那一行是「节点进度 … 1/1」。
	// 要的那个数只在传输面板上——它由聚合器回填；命令那一行的台数是工人自己数的，
	// 收口那一行调用删掉它照样是 1。所以按标签挑行，不整段搜。
	label := message.NodeProgressLabel()
	var found bool
	for _, line := range strings.Split(panel.String(), "\n") {
		if strings.Contains(line, label) && strings.Contains(line, "1/1") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("传输面板最后一帧没写「1/1 台完成」——收口没把结果交给聚合器？\n%s", panel.String())
	}
}
