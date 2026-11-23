package execute

// 失败回滚走收尾那一段（ADR-0065）：删法与超时收尾同一个动作，只受自己那份预算约束。
//
// 触发回滚的路只剩「写到中途失败」——撞名已经拦在开传之前。这里让目标机上的子目录位置
// 被一个普通文件占着：第一个文件写得进去，第二个建不了目录，于是回滚删第一个。

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sshfleet2/internal/contract"
)

// blockedSubdirPlan 造这台假目标机上的占位文件，并给出一张两个文件的上传纸：
// `a.txt` 写得进去，`sub/b.txt` 建目录时撞上占位文件。
func blockedSubdirPlan(t *testing.T, ft *fakeTarget, node contract.Node, rel string, run contract.RunParams) *contract.Plan {
	t.Helper()

	remoteDir := ft.path(rel)
	if err := os.MkdirAll(ft.local(remoteDir), 0o755); err != nil {
		t.Fatalf("建目标机上的落点目录失败：%v", err)
	}
	if err := os.WriteFile(filepath.Join(ft.local(remoteDir), "sub"), []byte("blocker"), 0o644); err != nil {
		t.Fatalf("造占位文件失败：%v", err)
	}

	local := t.TempDir()
	if err := os.MkdirAll(filepath.Join(local, "sub"), 0o755); err != nil {
		t.Fatalf("建本地子目录失败：%v", err)
	}
	files := make([]contract.UploadFile, 0, 2)
	for _, name := range []string{"a.txt", "sub/b.txt"} {
		p := filepath.Join(local, name)
		body := []byte("body of " + name)
		if err := os.WriteFile(p, body, 0o644); err != nil {
			t.Fatalf("写本地文件失败：%v", err)
		}
		files = append(files, contract.UploadFile{Path: p, Rel: name, Size: int64(len(body))})
	}

	return &contract.Plan{
		Mode: contract.ModeUpload,
		Task: contract.Task{
			Path:   remoteDir,
			Upload: &contract.UploadTask{Source: local, Files: files},
		},
		Nodes: []contract.Node{node},
		Run:   run,
	}
}

// 回滚删得干净：报「写进去的都删了」，目标机上不留本次的东西。
//
// 这张纸**不填收尾预算**（`timeout_connect` 缺席）：预算为 0 表示不设上限，不是「来不及
// 清理就不清」——直接拼纸的调用方都走这一条。
func TestRunUploadRollbackRemovesThisRunsFiles(t *testing.T) {
	ft := startFakeTarget(t)
	node := fakeNode(t, ft)
	p := blockedSubdirPlan(t, ft, node, "rollback", contract.RunParams{TimeoutExecute: 30})

	paper, _ := runWithLandings(t, p)
	it := paper.Items[0]
	remoteDir := p.Task.Path

	if !strings.Contains(it.ErrorText, "sshfleet.transfer.rolled_back: 1 of 1 written files removed") {
		t.Fatalf("回滚该照实说删了几个，实际 %q", it.ErrorText)
	}
	if it.Transfer.SuccessFiles != 1 || it.Transfer.FailFiles != 1 {
		t.Fatalf("四个数不对：%+v", it.Transfer)
	}
	if _, err := os.Stat(filepath.Join(ft.local(remoteDir), "a.txt")); err == nil {
		t.Fatal("回滚没删掉本次写进去的那个")
	}
	// 占位文件是别人放的，一个字节都不许动。
	if got, err := os.ReadFile(filepath.Join(ft.local(remoteDir), "sub")); err != nil || string(got) != "blocker" {
		t.Fatalf("占位文件被动了：%q（%v）", got, err)
	}
}

// 回滚卡在删除上：收尾预算用完就停，报「没清干净」并把还留着的路径列出来。
func TestRunUploadRollbackStopsWhenDeleteStalls(t *testing.T) {
	ft := startFakeTarget(t)
	ft.stallExecMatching("rm -f")
	node := fakeNode(t, ft)
	p := blockedSubdirPlan(t, ft, node, "rollback-stall",
		contract.RunParams{TimeoutExecute: 30, TimeoutTransfer: 30, TimeoutConnect: 1})

	paper, _ := runWithLandings(t, p)
	it := paper.Items[0]
	remoteDir := p.Task.Path

	if !strings.Contains(it.ErrorText, "sshfleet.transfer.rollback_partial") {
		t.Fatalf("删不掉该报没清干净，实际 %q", it.ErrorText)
	}
	if !strings.Contains(it.ErrorText, "left: "+remoteDir+"/a.txt") {
		t.Fatalf("该列出还留着的路径，实际 %q", it.ErrorText)
	}
	if _, err := os.Stat(filepath.Join(ft.local(remoteDir), "a.txt")); err != nil {
		t.Fatalf("删除卡住时那个文件该还在：%v", err)
	}
	// 回滚是「失败之后的收尾」，不是这一台传输到点：两者互斥（ADR-0065）。
	if it.TimedOut {
		t.Fatalf("回滚卡住不该记成传输超时：%q", it.ErrorText)
	}
}

// 回滚跑在传输预算之外：传输预算先到期，清理照样按自己那份跑完，也不会被记成超时。
func TestRunUploadRollbackOutlivesTransferBudget(t *testing.T) {
	ft := startFakeTarget(t)
	ft.stallExecMatching("rm -f")
	node := fakeNode(t, ft)
	// 传输预算 1 秒、收尾预算 2 秒：删除卡住的时间长过传输预算，清理仍由自己那份兜着。
	p := blockedSubdirPlan(t, ft, node, "rollback-budget",
		contract.RunParams{TimeoutExecute: 30, TimeoutTransfer: 1, TimeoutConnect: 2})

	paper, _ := runWithLandings(t, p)
	it := paper.Items[0]

	if it.TimedOut {
		t.Fatalf("回滚不吃传输预算，不该记超时：%q", it.ErrorText)
	}
	if strings.Contains(it.ErrorText, "sshfleet.transfer.timeout") {
		t.Fatalf("回滚期间不该冒出传输超时那一句：%q", it.ErrorText)
	}
	if !strings.Contains(it.ErrorText, "sshfleet.transfer.rollback_partial") {
		t.Fatalf("清理按自己那份预算停手，该报没清干净，实际 %q", it.ErrorText)
	}
}
