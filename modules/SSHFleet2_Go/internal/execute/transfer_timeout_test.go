package execute

// 传输预算到点时的收场（ADR-0063 决定 6、ADR-0065）。
//
// 怎么造「卡住」：假目标机的 SFTP 服务端在版本交换之后不再应答，客户端的那次发起就永远
// 等不到回应。到点时必须有人去关通道——这就是被测的那件事。

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"sshfleet2/internal/contract"
)

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
