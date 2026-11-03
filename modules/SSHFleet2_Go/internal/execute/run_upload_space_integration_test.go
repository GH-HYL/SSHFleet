//go:build integration

package execute

// 空间预检（13 号单）：装不下就停在写第一个文件之前，目标机上不留本次的任何东西。
//
// 现场靠容器那块 64MB 小卷（`--tmpfs /mnt/small64:size=64m`，起法见
// `docs/测试环境使用指南.md` 第三节）：造一个比它大的本地文件，让预检非拒不可。

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sshfleet2/internal/contract"
)

// TestRunUploadBlockedByTargetSpace 目标分区装不下：一个文件都不写，报文说清「没写」。
func TestRunUploadBlockedByTargetSpace(t *testing.T) {
	node, _ := integrationTarget(t)
	remote := remoteTempDir(t, node, remoteWritable)

	// 70MB 稀疏文件：文件系统上不真占空间，size 那一格照实是 70MB。
	big := filepath.Join(t.TempDir(), "big.bin")
	f, err := os.Create(big)
	if err != nil {
		t.Fatalf("造大文件失败：%v", err)
	}
	if err := f.Truncate(70 << 20); err != nil {
		t.Fatalf("拉大文件失败：%v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("关文件失败：%v", err)
	}

	p := planFor(t, []contract.Node{node}, "-u", big, "-p", remote)
	item := runOne(t, context.Background(), p)

	if !strings.Contains(item.ErrorText, "sshfleet.transfer.target_space_short: ") {
		t.Fatalf("该报目标分区装不下，实际 %q", item.ErrorText)
	}
	if !strings.Contains(item.ErrorText, "no file written") {
		t.Fatalf("报文里该说清「一个文件都没写」，实际 %q", item.ErrorText)
	}
	if item.Transfer.SuccessFiles != 0 || item.Transfer.FailFiles != 1 {
		t.Fatalf("被预检挡下就不该有成功项，实际 %+v", item.Transfer)
	}
	// 目标机上零残留——这正是「写第一个文件之前判」的意义。
	if left := remoteRun(t, node, "find "+remote+" -type f | wc -l"); left.Output != "0" {
		t.Fatalf("被挡下的那次在目标机上留了东西：%q", left.Output)
	}
}

// TestRunUploadSmallFitsSmallVolume 同一块小卷，装得下就照传——预检不该把正常上传挡掉。
func TestRunUploadSmallFitsSmallVolume(t *testing.T) {
	node, _ := integrationTarget(t)
	remote := remoteTempDir(t, node, remoteWritable)

	local := filepath.Join(t.TempDir(), "small.txt")
	if err := os.WriteFile(local, []byte("fits"), 0o644); err != nil {
		t.Fatalf("造小文件失败：%v", err)
	}

	p := planFor(t, []contract.Node{node}, "-u", local, "-p", remote)
	item := runOne(t, context.Background(), p)

	if item.ErrorText != "" {
		t.Fatalf("装得下却报错：%q", item.ErrorText)
	}
	if item.Transfer.SuccessFiles != 1 {
		t.Fatalf("该传成 1 个，实际 %+v", item.Transfer)
	}
	if got := remoteRun(t, node, "cat "+remote+"/small.txt"); got.Output != "fits" {
		t.Fatalf("内容不对：%q", got.Output)
	}
}
