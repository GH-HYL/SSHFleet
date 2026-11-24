package execute

// 链路半途断掉时，目标机上那个名字还归不归本账管。
//
// 第一趟写了一半，同一条死链上删也删不掉，于是它被交上来当残留；第二趟连建文件都建不出来，
// 交回空手。目标机上那个名字还在——报文不能因为第二趟没碰它，就把第一趟的交待冲掉。

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// 两趟都没能把那个名字收干净：报文要点名，目标机上确实留着半截文件。
func TestRunUploadReportsLeftoverTheRetryNeverTouched(t *testing.T) {
	ft := startFakeTarget(t)
	// 读到 256KB 就把这条 SFTP 通道断掉：那一趟写不完，之后的每一趟也发起不了。
	ft.dropSftpAfter(256 << 10)
	node := fakeNode(t, ft)

	local := t.TempDir()
	name := "big.bin"
	body := bytes.Repeat([]byte("x"), 1<<20)
	if err := os.WriteFile(filepath.Join(local, name), body, 0o644); err != nil {
		t.Fatalf("写本地文件失败：%v", err)
	}

	remoteDir := ft.path("dropped")
	if err := os.MkdirAll(ft.local(remoteDir), 0o755); err != nil {
		t.Fatalf("建目标机上的落点目录失败：%v", err)
	}
	p := uploadPlanWith(t, node, local, remoteDir, name)
	p.Run.TransferRetry = 1 // 两趟：第一趟断在中途，第二趟连建都建不出来

	paper, _ := runWithLandings(t, p)
	it := paper.Items[0]

	if !strings.Contains(it.ErrorText, "sshfleet.transfer.partial_left") {
		t.Fatalf("目标机上那个半截文件还在，报文该点名，实际 %q", it.ErrorText)
	}
	if !strings.Contains(it.ErrorText, remoteDir+"/"+name) {
		t.Fatalf("点名的路径不对，实际 %q", it.ErrorText)
	}
	info, err := os.Stat(filepath.Join(ft.local(remoteDir), name))
	if err != nil {
		t.Fatalf("目标机上该留着那个半截文件：%v", err)
	}
	if info.Size() == 0 || info.Size() == int64(len(body)) {
		t.Fatalf("该停在中途、不是空文件也不是完整文件，实际 %d 字节", info.Size())
	}
	if it.Transfer.SuccessFiles != 0 || it.Transfer.FailFiles != 1 {
		t.Fatalf("四个数不对：%+v", it.Transfer)
	}
}
