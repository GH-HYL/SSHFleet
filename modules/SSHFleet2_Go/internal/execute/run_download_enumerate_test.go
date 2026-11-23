package execute

// 下载枚举与开传前的空目录假定（ADR-0063 决定 4）。
//
// 枚举住 `run_download.go` 的那一段；落点是不是空目录不在这里判——解析站在开跑之前挡下，
// 执行站按「一定是空目录」办。
// 接缝仍是 `execute.Run`——递一张纸、收一张纸；假目标机只加开关，不另造假件。

import (
	"os"
	"path/filepath"
	"testing"

	"sshfleet2/internal/contract"
)

// downloadPlan 一张下载的纸：远端一个路径、本机一个落地目录。
func downloadPlan(node contract.Node, remote, local string) *contract.Plan {
	return &contract.Plan{
		Mode: contract.ModeDownload,
		Task: contract.Task{
			Path:     local,
			Download: &contract.DownloadTask{RemotePath: remote},
		},
		Nodes: []contract.Node{node},
		Run:   contract.RunParams{TimeoutExecute: 30},
	}
}

// 目标机不认 `-printf`：改走 `find -print0`，大小由客户端自己补，下载照样成。
func TestRunDownloadFallsBackWhenFindRefusesPrintf(t *testing.T) {
	ft := startFakeTarget(t)
	ft.refusePrintf()
	node := fakeNode(t, ft)

	remoteDir := ft.path("fallback")
	for rel, body := range map[string]string{"a.txt": "alpha\n", "sub/b.txt": "beta\n"} {
		full := filepath.Join(ft.local(remoteDir), filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("建目标机上的目录失败：%v", err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatalf("在目标机上造文件失败：%v", err)
		}
	}

	local := t.TempDir()
	paper, _ := runWithLandings(t, downloadPlan(node, remoteDir, local))
	it := paper.Items[0]

	if it.ErrorText != "" {
		t.Fatalf("退路该照常下完，实际报错原文 %q", it.ErrorText)
	}
	if it.Transfer.TotalFiles != 2 || it.Transfer.SuccessFiles != 2 || it.Transfer.FailFiles != 0 {
		t.Fatalf("四个数不对：%+v（明细：%s）", it.Transfer, it.Output)
	}
	// 退路那头枚举也记一条中间步骤，且是 0——退路是成功路径，不是失败。
	if len(it.Steps) != 2 || it.Steps[1].Name != contract.StepEnumerate {
		t.Fatalf("枚举该记一条中间步骤，实际 %+v", it.Steps)
	}
	if code := it.Steps[1].ExitCode; code == nil || *code != 0 {
		t.Fatalf("走退路也是成功枚举，退出码该是 0：%+v", it.Steps[1])
	}

	got, err := os.ReadFile(filepath.Join(local, node.IP, "sub", "b.txt"))
	if err != nil {
		t.Fatalf("退路没把子目录里的文件取回来：%v", err)
	}
	if string(got) != "beta\n" {
		t.Fatalf("取回的内容不对：%q", got)
	}
}

// 记录里的名字带换行：按 NUL 切，它仍是一条；按换行切会多出一条碎片。
//
// 一条真文件（落得下来）+ 一条多吐的怪记录（名字里带换行、远端没有这个文件）。
// 按 NUL 切是 2 个；按换行切，那条怪记录会被拆成两个文件，总数 3。
func TestRunDownloadKeepsRecordWithNewlineInName(t *testing.T) {
	ft := startFakeTarget(t)
	ft.appendFindRecord("F 4 weird\nF 4 ghost.txt")
	node := fakeNode(t, ft)

	remoteDir := ft.path("newline")
	if err := os.MkdirAll(ft.local(remoteDir), 0o755); err != nil {
		t.Fatalf("建目标机上的目录失败：%v", err)
	}
	if err := os.WriteFile(filepath.Join(ft.local(remoteDir), "a.txt"), []byte("alpha\n"), 0o644); err != nil {
		t.Fatalf("在目标机上造文件失败：%v", err)
	}

	local := t.TempDir()
	paper, _ := runWithLandings(t, downloadPlan(node, remoteDir, local))
	it := paper.Items[0]

	if it.Transfer.TotalFiles != 2 {
		t.Fatalf("名字里的换行不该拆记录：该 2 个文件，实际 %d（明细：%s）", it.Transfer.TotalFiles, it.Output)
	}
	if it.Transfer.SuccessFiles != 1 {
		t.Fatalf("那个真文件该下回来，实际 %+v（明细：%s）", it.Transfer, it.Output)
	}
	got, err := os.ReadFile(filepath.Join(local, node.IP, "a.txt"))
	if err != nil {
		t.Fatalf("真文件没落下来：%v", err)
	}
	if string(got) != "alpha\n" {
		t.Fatalf("取回的内容不对：%q", got)
	}
}
