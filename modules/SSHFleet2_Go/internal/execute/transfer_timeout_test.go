package execute

// 传输预算到点时的收场（ADR-0063 决定 6、ADR-0065）。
//
// 怎么造「卡住」：假目标机的 SFTP 服务端在版本交换之后不再应答，客户端的那次发起就永远
// 等不到回应。到点时必须有人去关通道——这就是被测的那件事。

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"sshfleet2/internal/contract"
)

// timeoutUploadPlan 一张会传到一半卡住的上传纸：本地一个较大的文件，目标机写到中途不再应答。
//
// 大文件是必须的：SFTP 的发送窗口让客户端一口气能把前面那一段塞进缓冲，文件不够大就根本
// 堵不住，一次「写到中途卡住」也造不出来。
func timeoutUploadPlan(t *testing.T, ft *fakeTarget, node contract.Node) (*contract.Plan, string) {
	t.Helper()

	localDir := t.TempDir()
	localFile := filepath.Join(localDir, "big.txt")
	body := bytes.Repeat([]byte("x"), 4<<20)
	if err := os.WriteFile(localFile, body, 0o644); err != nil {
		t.Fatalf("写本地文件失败：%v", err)
	}

	remoteDir := ft.path("upload-timeout")
	if err := os.MkdirAll(ft.local(remoteDir), 0o755); err != nil {
		t.Fatalf("建目标机上的落点目录失败：%v", err)
	}

	return &contract.Plan{
		Mode: contract.ModeUpload,
		Task: contract.Task{
			Path: remoteDir,
			Upload: &contract.UploadTask{
				Source: localDir,
				Files:  []contract.UploadFile{{Path: localFile, Rel: "big.txt", Size: int64(len(body))}},
			},
		},
		Nodes: []contract.Node{node},
		Run:   contract.RunParams{TimeoutExecute: 30, TimeoutTransfer: 1, TimeoutConnect: 5},
	}, filepath.Join(ft.local(remoteDir), "big.txt")
}

// 上传卡在预检那一次 Stat 上：到点收场、报「目标机上没有本次的东西」，不等满整个执行超时。
func TestRunUploadTimesOutWhenServerStopsAnswering(t *testing.T) {
	ft := startFakeTarget(t)
	ft.stallSftp()
	node := fakeNode(t, ft)

	localDir := t.TempDir()
	localFile := filepath.Join(localDir, "a.txt")
	if err := os.WriteFile(localFile, []byte("alpha\n"), 0o644); err != nil {
		t.Fatalf("写本地文件失败：%v", err)
	}

	remoteDir := ft.path("upload-stall")
	if err := os.MkdirAll(ft.local(remoteDir), 0o755); err != nil {
		t.Fatalf("建目标机上的落点目录失败：%v", err)
	}

	p := &contract.Plan{
		Mode: contract.ModeUpload,
		Task: contract.Task{
			Path: remoteDir,
			Upload: &contract.UploadTask{
				Source: localDir,
				Files:  []contract.UploadFile{{Path: localFile, Rel: "a.txt", Size: int64(len("alpha\n"))}},
			},
		},
		Nodes: []contract.Node{node},
		// 传输预算 1 秒：到点就该收场；执行超时给足，证明停下的是传输那一份。
		Run: contract.RunParams{TimeoutExecute: 30, TimeoutTransfer: 1, TimeoutConnect: 5},
	}

	start := time.Now()
	paper, _ := runWithLandings(t, p)
	elapsed := time.Since(start)
	it := paper.Items[0]

	if !it.TimedOut {
		t.Fatalf("该记超时，实际 %+v（错误原文 %q）", it.Transfer, it.ErrorText)
	}
	if it.Cancelled {
		t.Fatalf("超时与中断互斥，不该同时成立：%q", it.ErrorText)
	}
	if !strings.Contains(it.ErrorText, "sshfleet.transfer.timeout") {
		t.Fatalf("该报传输超时，实际 %q", it.ErrorText)
	}
	// 卡在预检：一个文件都没写过，所以是「没有本次的东西」那一档。
	if !strings.Contains(it.ErrorText, "no file left") {
		t.Fatalf("没写过文件时该说没有残留，实际 %q", it.ErrorText)
	}
	if it.Transfer.TotalFiles != 1 || it.Transfer.SuccessFiles != 0 || it.Transfer.FailFiles != 1 {
		t.Fatalf("四个数不对：%+v", it.Transfer)
	}
	if elapsed > 15*time.Second {
		t.Fatalf("1 秒的预算却等了 %v——到点之后没解开阻塞", elapsed)
	}
	if _, err := os.Stat(filepath.Join(ft.local(remoteDir), "a.txt")); err == nil {
		t.Fatal("卡在预检，目标机上不该有这个文件")
	}
}

// 上传写到中途卡住：到点之后真去清理，删干净时报「超时、目标机没有本次的东西」并跟一句回滚陈述。
func TestRunUploadCleansUpAfterTimeout(t *testing.T) {
	ft := startFakeTarget(t)
	ft.stallSftpAfter(256 << 10) // 目标机收下 256KB 之后不再应答
	node := fakeNode(t, ft)

	p, remoteFile := timeoutUploadPlan(t, ft, node)
	paper, _ := runWithLandings(t, p)
	it := paper.Items[0]

	if !it.TimedOut {
		t.Fatalf("该记超时，实际 %q", it.ErrorText)
	}
	if !strings.Contains(it.ErrorText, "sshfleet.transfer.timeout: ") {
		t.Fatalf("该报传输超时，实际 %q", it.ErrorText)
	}
	if !strings.Contains(it.ErrorText, "no file left") {
		t.Fatalf("删干净了该说目标机没有本次的东西，实际 %q", it.ErrorText)
	}
	if !strings.Contains(it.ErrorText, "sshfleet.transfer.rolled_back") {
		t.Fatalf("删干净了该跟一句回滚陈述，实际 %q", it.ErrorText)
	}
	if _, err := os.Stat(remoteFile); err == nil {
		t.Fatal("清理之后目标机上不该留着那个文件")
	}
}

// 上传写到中途卡住、清理也删不掉：分类那一档要说清还剩哪个路径。
func TestRunUploadReportsLeftoversWhenCleanupFails(t *testing.T) {
	ft := startFakeTarget(t)
	ft.stallSftpAfter(256 << 10)
	ft.refuseRemove()
	node := fakeNode(t, ft)

	p, remoteFile := timeoutUploadPlan(t, ft, node)
	paper, _ := runWithLandings(t, p)
	it := paper.Items[0]

	if !strings.Contains(it.ErrorText, "sshfleet.transfer.timeout_left") {
		t.Fatalf("没删干净该报 timeout_left，实际 %q", it.ErrorText)
	}
	if !strings.Contains(it.ErrorText, "0 of 1 written files removed") {
		t.Fatalf("该说清一个都没删掉，实际 %q", it.ErrorText)
	}
	if !strings.Contains(it.ErrorText, "left: ") {
		t.Fatalf("该列出还留着的路径，实际 %q", it.ErrorText)
	}
	if _, err := os.Stat(remoteFile); err != nil {
		t.Fatalf("清理失败时那个文件该还在：%v", err)
	}
}

// 清理那条命令根本发不出去（会话卡在请求上）：报「没来得及」，不假装删过。
func TestRunUploadReportsCleanupNotAttempted(t *testing.T) {
	ft := startFakeTarget(t)
	ft.stallSftpAfter(256 << 10)
	ft.stallExec()
	node := fakeNode(t, ft)

	p, remoteFile := timeoutUploadPlan(t, ft, node)
	paper, _ := runWithLandings(t, p)
	it := paper.Items[0]

	if !strings.Contains(it.ErrorText, "cleanup not attempted") {
		t.Fatalf("该报没来得及清理，实际 %q", it.ErrorText)
	}
	if !strings.Contains(it.ErrorText, "left: ") {
		t.Fatalf("没来得及清理时也要报全，实际 %q", it.ErrorText)
	}
	if _, err := os.Stat(remoteFile); err != nil {
		t.Fatalf("清理没跑成，那个文件该还在：%v", err)
	}
}

// 传到一半被 Ctrl+C、而这次写又卡在死链上：等不到文件边界，由传输预算到点解开；
// 记的是中断不是超时，清理照做（ADR-0065：中断不是清理的停止条件）。
func TestRunUploadCleansUpAfterCancel(t *testing.T) {
	ft := startFakeTarget(t)
	ft.stallSftpAfter(256 << 10)
	node := fakeNode(t, ft)

	p, remoteFile := timeoutUploadPlan(t, ft, node)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		time.Sleep(200 * time.Millisecond)
		cancel()
	}()

	it := runWithContext(t, ctx, p).Items[0]

	if !it.Cancelled {
		t.Fatalf("父 ctx 已取消，该记中断，实际 %q", it.ErrorText)
	}
	if it.TimedOut {
		t.Fatalf("超时与中断互斥，不该同时成立：%q", it.ErrorText)
	}
	if !strings.Contains(it.ErrorText, "sshfleet.abort.") {
		t.Fatalf("该报中断，实际 %q", it.ErrorText)
	}
	// 清理不看 ctx：它照做，删干净了才报 rolled_back。
	if !strings.Contains(it.ErrorText, "sshfleet.transfer.rolled_back") {
		t.Fatalf("中断之后清理也该做，实际 %q", it.ErrorText)
	}
	if _, err := os.Stat(remoteFile); err == nil {
		t.Fatal("清理之后目标机上不该留着那个文件")
	}
}
