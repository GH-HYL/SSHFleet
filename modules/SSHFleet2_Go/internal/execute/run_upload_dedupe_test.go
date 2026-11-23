package execute

// 上传的远端重名：开传之前整批问一次。
//
// 接缝是 `execute.Run`——递一张纸、收一张纸；判据只看纸上的四个数与报错原文，以及
// 目标机上真留下了什么。假目标机认那条循环（`[ -e ] || [ -L ]`），也有一个开关让这条
// 循环退非 0，用来验退回逐文件问的那一路。

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sshfleet2/internal/contract"
)

// uploadPlanWith 两个文件的纸：源在本机临时目录，落点在目标机上那个目录。
func uploadPlanWith(t *testing.T, node contract.Node, localDir, remoteDir string, names ...string) *contract.Plan {
	t.Helper()
	files := make([]contract.UploadFile, 0, len(names))
	for _, n := range names {
		p := filepath.Join(localDir, n)
		info, err := os.Stat(p)
		if err != nil {
			t.Fatalf("本机没有 %s：%v", n, err)
		}
		files = append(files, contract.UploadFile{Path: p, Rel: n, Size: info.Size()})
	}
	return &contract.Plan{
		Mode: contract.ModeUpload,
		Task: contract.Task{
			Path:   remoteDir,
			Upload: &contract.UploadTask{Source: localDir, Files: files},
		},
		Nodes: []contract.Node{node},
		Run:   contract.RunParams{TimeoutExecute: 30},
	}
}

// localFiles 在本机造出这几个文件，交回目录。
func localFiles(t *testing.T, names ...string) string {
	t.Helper()
	dir := t.TempDir()
	for _, n := range names {
		if err := os.WriteFile(filepath.Join(dir, n), []byte("local-"+n), 0o644); err != nil {
			t.Fatalf("写本机文件失败：%v", err)
		}
	}
	return dir
}

// 目标机上已经有同名文件：整批停在开传之前，一个字节都不写。
func TestRunUploadStopsBeforeWritingWhenRemoteExists(t *testing.T) {
	ft := startFakeTarget(t)
	node := fakeNode(t, ft)
	local := localFiles(t, "a.txt", "b.txt")

	remoteDir := ft.path("taken")
	if err := os.MkdirAll(ft.local(remoteDir), 0o755); err != nil {
		t.Fatalf("建目标机上的落点目录失败：%v", err)
	}
	const kept = "remote-keep\n"
	taken := filepath.Join(ft.local(remoteDir), "a.txt")
	if err := os.WriteFile(taken, []byte(kept), 0o644); err != nil {
		t.Fatalf("在目标机上预置同名文件失败：%v", err)
	}

	paper, _ := runWithLandings(t, uploadPlanWith(t, node, local, remoteDir, "a.txt", "b.txt"))
	it := paper.Items[0]

	if !strings.Contains(it.ErrorText, "sshfleet.transfer.remote_exists") {
		t.Fatalf("该报远端重名，实际 %q", it.ErrorText)
	}
	if !strings.Contains(it.ErrorText, "a.txt") || strings.Contains(it.ErrorText, "b.txt") {
		t.Fatalf("只该点名撞上的那个，实际 %q", it.ErrorText)
	}
	// 命中的是开传之前：一个都没传，四个数按整批失败记。
	if it.Transfer.TotalFiles != 2 || it.Transfer.FailFiles != 2 || it.Transfer.SuccessFiles != 0 {
		t.Fatalf("四个数不对：%+v（错误原文 %q）", it.Transfer, it.ErrorText)
	}
	if it.Transfer.TotalBytes != 0 {
		t.Fatalf("零写入却记了字节数：%d", it.Transfer.TotalBytes)
	}

	// 目标机上：原来那个文件一个字节没变，另一个文件根本没出现。
	got, err := os.ReadFile(taken)
	if err != nil || string(got) != kept {
		t.Fatalf("目标机上的同名文件被动了：%q（%v）", got, err)
	}
	if _, err := os.Stat(filepath.Join(ft.local(remoteDir), "b.txt")); err == nil {
		t.Fatal("被拦下时不该写进别的文件")
	}
}

// 名字被一个断链占着：仍然算重名。
//
// 这条钉的是 `-L`：`test -e` 与 SFTP `Stat` 都跟随软链接，断链在它们眼里是「不存在」，
// 于是紧接着的 Create 会顺着链接写到别处去。
func TestRunUploadTreatsDanglingLinkAsTaken(t *testing.T) {
	ft := startFakeTarget(t)
	node := fakeNode(t, ft)
	local := localFiles(t, "a.txt")

	remoteDir := ft.path("dangling")
	if err := os.MkdirAll(ft.local(remoteDir), 0o755); err != nil {
		t.Fatalf("建目标机上的落点目录失败：%v", err)
	}
	link := filepath.Join(ft.local(remoteDir), "a.txt")
	if err := os.Symlink("nowhere", link); err != nil {
		t.Skipf("这台造不出软链接：%v", err)
	}

	paper, _ := runWithLandings(t, uploadPlanWith(t, node, local, remoteDir, "a.txt"))
	it := paper.Items[0]

	if !strings.Contains(it.ErrorText, "sshfleet.transfer.remote_exists") {
		t.Fatalf("断链占着名字也该算重名，实际 %q", it.ErrorText)
	}
	// 没顺着链接写进去：`nowhere` 不该出现。这条才是判据——只跟随链接的谓词会放过它，
	// 紧接着的 Create 就在那个位置建出文件来。
	if _, err := os.Lstat(filepath.Join(ft.local(remoteDir), "nowhere")); err == nil {
		t.Fatal("顺着断链把文件写到别处去了")
	}
	if info, err := os.Lstat(link); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("目标机上那个名字该仍是软链接：%v（%v）", info, err)
	}
}

// 壳不认那条循环（退非 0）：退回逐文件问，重名照样拦得住。
func TestRunUploadFallsBackWhenDedupeLoopRefused(t *testing.T) {
	ft := startFakeTarget(t)
	ft.refuseDedupe()
	node := fakeNode(t, ft)
	local := localFiles(t, "a.txt")

	remoteDir := ft.path("fallback")
	if err := os.MkdirAll(ft.local(remoteDir), 0o755); err != nil {
		t.Fatalf("建目标机上的落点目录失败：%v", err)
	}
	if err := os.WriteFile(filepath.Join(ft.local(remoteDir), "a.txt"), []byte("remote\n"), 0o644); err != nil {
		t.Fatalf("在目标机上预置同名文件失败：%v", err)
	}

	paper, _ := runWithLandings(t, uploadPlanWith(t, node, local, remoteDir, "a.txt"))
	it := paper.Items[0]

	if !strings.Contains(it.ErrorText, "sshfleet.transfer.remote_exists") {
		t.Fatalf("退路也该拦住重名，实际 %q", it.ErrorText)
	}
	if it.Transfer.FailFiles != 1 || it.Transfer.SuccessFiles != 0 {
		t.Fatalf("四个数不对：%+v", it.Transfer)
	}
}
