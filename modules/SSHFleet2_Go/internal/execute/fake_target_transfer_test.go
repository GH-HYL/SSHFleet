package execute

// 03 号工单：上传与下载从入口真跑完一趟。
//
// 远端根是假目标机的临时目录，所以「远端路径」用 `path()` 拼成 `/C:/…` 那种形状；
// 目标机上要先把上传的落点目录建出来——工具不建 `-p` 指的目录（那是解析站的活）。

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sshfleet2/internal/contract"
)

// fakeNode 把假目标机的地址包成清单里的一台。
func fakeNode(t *testing.T, ft *fakeTarget) contract.Node {
	t.Helper()
	host, port, err := net.SplitHostPort(ft.addr)
	if err != nil {
		t.Fatalf("拆地址失败：%v", err)
	}
	return contract.Node{IP: host, Port: port, User: "tester", Password: "secret"}
}

// runWithLandings 跑一趟，交回纸与终端输出档案的正文。
func runWithLandings(t *testing.T, p *contract.Plan) (contract.ResultPaper, string) {
	t.Helper()
	dir := t.TempDir()
	termPath := filepath.Join(dir, "terminal-output.txt")
	paper, fail := Run(context.Background(), p, contract.Landings{
		Terminal: termPath,
		ExecLog:  filepath.Join(dir, "SSHFleetExec.log"),
	}, nil)
	if fail != nil {
		t.Fatalf("不该起不来：%+v", fail)
	}
	body, err := os.ReadFile(termPath)
	if err != nil {
		t.Fatalf("读终端输出档案失败：%v", err)
	}
	return paper, string(body)
}

func TestRunUploadOverFakeTarget(t *testing.T) {
	ft := startFakeTarget(t)
	node := fakeNode(t, ft)

	const body = "alpha\n"
	localDir := t.TempDir()
	localFile := filepath.Join(localDir, "a.txt")
	if err := os.WriteFile(localFile, []byte(body), 0o644); err != nil {
		t.Fatalf("写本地文件失败：%v", err)
	}

	remoteDir := ft.path("upload")
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

	paper, term := runWithLandings(t, p)
	if len(paper.Items) != 1 {
		t.Fatalf("该交回一台的事实，实际 %d 条", len(paper.Items))
	}
	it := paper.Items[0]
	if !it.Connected {
		t.Fatalf("这一台该连上了：%q", it.ErrorText)
	}
	if it.Transfer.TotalFiles != 1 || it.Transfer.SuccessFiles != 1 || it.Transfer.FailFiles != 0 {
		t.Fatalf("四个数不对：%+v（错误原文 %q）", it.Transfer, it.ErrorText)
	}
	if it.Transfer.TotalBytes != int64(len(body)) {
		t.Fatalf("字节数 want %d，got %d", len(body), it.Transfer.TotalBytes)
	}

	// 目标机上真有这个文件，内容对得上。
	got, err := os.ReadFile(filepath.Join(ft.local(remoteDir), "a.txt"))
	if err != nil {
		t.Fatalf("目标机上没这个文件：%v", err)
	}
	if string(got) != body {
		t.Fatalf("目标机上的内容 want %q，got %q", body, got)
	}

	if !strings.Contains(term, node.IP) {
		t.Fatalf("终端输出档案里没有这一台：%q", term)
	}
}

func TestRunDownloadOverFakeTarget(t *testing.T) {
	ft := startFakeTarget(t)
	node := fakeNode(t, ft)

	const body = "beta\n"
	remoteDir := ft.path("download")
	if err := os.MkdirAll(ft.local(remoteDir), 0o755); err != nil {
		t.Fatalf("建目标机上的目录失败：%v", err)
	}
	if err := os.WriteFile(filepath.Join(ft.local(remoteDir), "b.txt"), []byte(body), 0o644); err != nil {
		t.Fatalf("在目标机上造文件失败：%v", err)
	}

	localDir := t.TempDir()
	p := &contract.Plan{
		Mode: contract.ModeDownload,
		Task: contract.Task{
			Path:     localDir,
			Download: &contract.DownloadTask{RemotePath: filepath.ToSlash(remoteDir) + "/b.txt"},
		},
		Nodes: []contract.Node{node},
		Run:   contract.RunParams{TimeoutExecute: 30},
	}

	paper, term := runWithLandings(t, p)
	if len(paper.Items) != 1 {
		t.Fatalf("该交回一台的事实，实际 %d 条", len(paper.Items))
	}
	it := paper.Items[0]
	if !it.Connected {
		t.Fatalf("这一台该连上了：%q", it.ErrorText)
	}
	if it.Transfer.TotalFiles != 1 || it.Transfer.SuccessFiles != 1 || it.Transfer.FailFiles != 0 {
		t.Fatalf("四个数不对：%+v（错误原文 %q）", it.Transfer, it.ErrorText)
	}

	// 下载落地在「本地目录／这台 IP／」下面（远端层级照旧）。
	got, err := os.ReadFile(filepath.Join(localDir, node.IP, "b.txt"))
	if err != nil {
		t.Fatalf("本机没取回文件：%v", err)
	}
	if string(got) != body {
		t.Fatalf("取回的内容 want %q，got %q", body, got)
	}

	if !strings.Contains(term, node.IP) {
		t.Fatalf("终端输出档案里没有这一台：%q", term)
	}
}

// 下载：枚举给的大小与真实内容不符——重试用完就失败，分类是「大小不符」，本机不留半成品。
//
// 只造得出「一直不符」这一种：期望大小来自枚举，重试不会再枚举一次（那是这一格的设计），
// 所以「第一次不符、第二次对上」在本机造不出来——真实文件系统写不短，服务端也没有截断开关。
func TestRunDownloadFailsWhenSizeNeverMatches(t *testing.T) {
	ft := startFakeTarget(t)
	node := fakeNode(t, ft)

	remoteDir := ft.path("mismatch-always")
	if err := os.MkdirAll(ft.local(remoteDir), 0o755); err != nil {
		t.Fatalf("建目标机上的目录失败：%v", err)
	}
	if err := os.WriteFile(filepath.Join(ft.local(remoteDir), "b.txt"), []byte("beta\n"), 0o644); err != nil {
		t.Fatalf("在目标机上造文件失败：%v", err)
	}
	ft.lieAboutSize(100, 99)

	localDir := t.TempDir()
	p := &contract.Plan{
		Mode: contract.ModeDownload,
		Task: contract.Task{
			Path:     localDir,
			Download: &contract.DownloadTask{RemotePath: filepath.ToSlash(remoteDir)},
		},
		Nodes: []contract.Node{node},
		Run:   contract.RunParams{TimeoutExecute: 30, TransferRetry: 1},
	}

	paper, _ := runWithLandings(t, p)
	it := paper.Items[0]
	if !strings.Contains(it.ErrorText, "sshfleet.transfer.size_mismatch") {
		t.Fatalf("该报大小不符，实际 %q", it.ErrorText)
	}
	if it.Transfer.FailFiles != 1 {
		t.Fatalf("该计一个失败，实际 %+v", it.Transfer)
	}
	if _, err := os.Stat(filepath.Join(localDir, node.IP, "b.txt")); err == nil {
		t.Fatal("大小不符的文件该删掉，本机不该留着")
	}
}
