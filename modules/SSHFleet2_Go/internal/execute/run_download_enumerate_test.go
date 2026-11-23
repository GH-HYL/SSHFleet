package execute

// 下载枚举与开传前的本机重名（ADR-0063 决定 4、决定 7）。
//
// 两件都住 `run_download.go` 的同一段：先枚举、再查本机落点，然后才开传。
// 接缝仍是 `execute.Run`——递一张纸、收一张纸；假目标机只加开关，不另造假件。

import (
	"os"
	"path/filepath"
	"strings"
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

// 本机已有同名文件：整批拦下，报名字、一个字节都不写。
func TestRunDownloadRefusesExistingLocalFile(t *testing.T) {
	ft := startFakeTarget(t)
	node := fakeNode(t, ft)

	remoteDir := ft.path("conflict")
	if err := os.MkdirAll(ft.local(remoteDir), 0o755); err != nil {
		t.Fatalf("建目标机上的目录失败：%v", err)
	}
	for _, name := range []string{"a.txt", "b.txt"} {
		if err := os.WriteFile(filepath.Join(ft.local(remoteDir), name), []byte("remote-"+name), 0o644); err != nil {
			t.Fatalf("在目标机上造文件失败：%v", err)
		}
	}

	local := t.TempDir()
	ipDir := filepath.Join(local, node.IP)
	if err := os.MkdirAll(ipDir, 0o755); err != nil {
		t.Fatalf("建本机落地目录失败：%v", err)
	}
	const kept = "local-keep"
	if err := os.WriteFile(filepath.Join(ipDir, "a.txt"), []byte(kept), 0o644); err != nil {
		t.Fatalf("预置本机同名文件失败：%v", err)
	}

	paper, _ := runWithLandings(t, downloadPlan(node, remoteDir, local))
	it := paper.Items[0]

	if !strings.Contains(it.ErrorText, "sshfleet.transfer.local_exists") {
		t.Fatalf("该报本机已存在，实际 %q", it.ErrorText)
	}
	if !strings.Contains(it.ErrorText, "a.txt") {
		t.Fatalf("该点名是哪个文件，实际 %q", it.ErrorText)
	}
	// 一个文件都没开始传：整批计失败。
	if it.Transfer.TotalFiles != 2 || it.Transfer.FailFiles != 2 || it.Transfer.SuccessFiles != 0 {
		t.Fatalf("四个数不对：%+v（错误原文 %q）", it.Transfer, it.ErrorText)
	}

	// 本机那份一个字节没变，也没有多出别的文件。
	got, err := os.ReadFile(filepath.Join(ipDir, "a.txt"))
	if err != nil || string(got) != kept {
		t.Fatalf("本机同名文件被改了：%q（%v）", got, err)
	}
	entries, err := os.ReadDir(ipDir)
	if err != nil {
		t.Fatalf("读本机落地目录失败：%v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("被拦下时不该多写文件，实际目录里有 %d 个", len(entries))
	}

	// 远端那两个文件仍在：这一趟只读不写。
	for _, name := range []string{"a.txt", "b.txt"} {
		if _, err := os.Stat(filepath.Join(ft.local(remoteDir), name)); err != nil {
			t.Fatalf("远端 %s 被动了：%v", name, err)
		}
	}
}
