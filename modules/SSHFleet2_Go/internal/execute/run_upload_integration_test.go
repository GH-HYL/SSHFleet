//go:build integration

package execute

// 对着真机器上传（12 号单）：层级保留、撞名不覆盖、失败只删本次写进去的那些。
//
// 现场都在远端可写目录里造（`/mnt/small64/w`，容器起法见 `docs/测试环境使用指南.md`），
// 每条用例自己建一个带时间戳的子目录，跑完删掉。

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sshfleet2/internal/contract"
)

// remoteWritable 是远端可写根（小容量卷上的那个 `w`）。
const remoteWritable = "/mnt/small64/w"

// localTree 造一棵本地目录树：一层文件 + 一层嵌套，交回根路径。
func localTree(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "sub"), 0o755); err != nil {
		t.Fatalf("建本地子目录失败：%v", err)
	}
	for path, body := range map[string]string{
		filepath.Join(root, "a.txt"):        "alpha",
		filepath.Join(root, "sub", "b.txt"): "beta",
		filepath.Join(root, "sub", "c.txt"): "gamma",
	} {
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatalf("写本地文件失败：%v", err)
		}
	}
	return root
}

// TestRunUploadKeepsTree 目录上传：层级留住，四个数对得上，明细进 Output 而不是屏幕一句话。
func TestRunUploadKeepsTree(t *testing.T) {
	node, _ := integrationTarget(t)
	remote := remoteTempDir(t, node, remoteWritable)
	local := localTree(t)

	p := planFor(t, []contract.Node{node}, "-u", local, "-p", remote)
	item := runOne(t, context.Background(), p)

	if !item.Connected {
		t.Fatalf("应能连上，错误原文 %q", item.ErrorText)
	}
	if item.ErrorText != "" {
		t.Fatalf("整批传成了不该有报错原文，实际 %q", item.ErrorText)
	}
	if item.Transfer.TotalFiles != 3 || item.Transfer.SuccessFiles != 3 || item.Transfer.FailFiles != 0 {
		t.Fatalf("四个数不对：%+v", item.Transfer)
	}
	// 明细首行是四个数（总数／成功／失败），不上屏的那一段落在这里。
	if want := "total_files=3, success_files=3, failed_files=0"; !strings.HasPrefix(item.Output, want) {
		t.Fatalf("明细首行应是 %q，实际：\n%s", want, item.Output)
	}

	// 层级与内容都要在：传上去的是同名同路径，不是把树压平。
	for _, probe := range []struct{ rel, want string }{
		{"a.txt", "alpha"},
		{"sub/b.txt", "beta"},
		{"sub/c.txt", "gamma"},
	} {
		got := remoteRun(t, node, "cat "+remote+"/"+probe.rel)
		if got.ExitCode == nil || *got.ExitCode != 0 {
			t.Fatalf("%s 没传上去：退出码 %v，原文 %q", probe.rel, got.ExitCode, got.ErrorText)
		}
		if got.Output != probe.want {
			t.Fatalf("%s 的内容不对：期望 %q，实际 %q", probe.rel, probe.want, got.Output)
		}
	}
}

// TestRunUploadRollsBackOnExisting 撞重名：不覆盖别人的文件，本次已写进去的删干净。
//
// a.txt 先写成功、b.txt 撞名，回滚只该删 a.txt；b.txt 是别人放的，一个字节都不许动。
func TestRunUploadRollsBackOnExisting(t *testing.T) {
	node, _ := integrationTarget(t)
	remote := remoteTempDir(t, node, remoteWritable)
	local := localTree(t)

	// 先放一个同名文件：内容与本地那份不同，才验得出「没被覆盖」。
	if item := remoteRun(t, node, "mkdir -p "+remote+"/sub && printf original > "+remote+"/sub/b.txt"); item.ExitCode == nil || *item.ExitCode != 0 {
		t.Fatalf("造重名文件失败：%+v", item)
	}
	if item := remoteRun(t, node, "test -f "+remote+"/a.txt"); item.ExitCode == nil || *item.ExitCode == 0 {
		t.Fatalf("a.txt 事前不该存在：%+v", item)
	}

	p := planFor(t, []contract.Node{node}, "-u", local, "-p", remote)
	item := runOne(t, context.Background(), p)

	if item.Transfer.SuccessFiles != 1 || item.Transfer.FailFiles != 1 {
		t.Fatalf("写成一个、撞名一个，实际 %+v（输出：%s）", item.Transfer, item.Output)
	}
	if !strings.Contains(item.ErrorText, "sshfleet.transfer.remote_exists: ") {
		t.Fatalf("报错原文该说撞名，实际 %q", item.ErrorText)
	}
	if !strings.Contains(item.ErrorText, "sshfleet.transfer.rolled_back: ") {
		t.Fatalf("回滚之后要照实说清，实际 %q", item.ErrorText)
	}
	if !strings.Contains(item.Output, "sub/b.txt: 上传失败 - ") {
		t.Fatalf("明细里该有那一行失败项，实际：\n%s", item.Output)
	}

	// 别人的文件原样：一个字节都没被覆盖。
	if got := remoteRun(t, node, "cat "+remote+"/sub/b.txt"); got.Output != "original" {
		t.Fatalf("重名文件被动了：%q", got.Output)
	}
	// 本次写成功的那个已经删掉：目标机上不留半套。
	if left := remoteRun(t, node, "test -e "+remote+"/a.txt"); left.ExitCode == nil || *left.ExitCode == 0 {
		t.Fatalf("回滚没删掉 a.txt：%+v", left)
	}
}

// TestRunUploadViaSudo 走提权路径：先写暂存目录再 mv 到位，暂存目录用完清掉。
func TestRunUploadViaSudo(t *testing.T) {
	node, _ := integrationTarget(t)
	remote := remoteTempDir(t, node, remoteWritable)
	local := localTree(t)

	p := planFor(t, []contract.Node{node}, "--sudo", "-u", local, "-p", remote)
	if !p.Run.Sudo {
		t.Fatal("--sudo 没落到纸上")
	}
	item := runOne(t, context.Background(), p)

	if item.ErrorText != "" {
		t.Fatalf("sudo 路径传成了不该有报错原文，实际 %q", item.ErrorText)
	}
	if item.Transfer.SuccessFiles != 3 {
		t.Fatalf("三个文件都该传成，实际 %+v", item.Transfer)
	}
	if got := remoteRun(t, node, "cat "+remote+"/sub/b.txt"); got.Output != "beta" {
		t.Fatalf("sudo 路径落位不对：%q", got.Output)
	}
	// 暂存目录不留哑巴残留：里面一个文件都不该剩（根目录本身空着无妨，下次开工先整体清）。
	if left := remoteRun(t, node, "find "+tmpRoot+" -type f 2>/dev/null | wc -l"); left.Output != "0" {
		t.Fatalf("暂存目录里还剩文件：%q", left.Output)
	}
}

// TestRunUploadRemoteMissing 目标目录不存在：停在原地，一个文件都没写。
func TestRunUploadRemoteMissing(t *testing.T) {
	node, _ := integrationTarget(t)
	local := localTree(t)
	missing := remoteWritable + "/definitely-not-there"

	p := planFor(t, []contract.Node{node}, "-u", local, "-p", missing)
	item := runOne(t, context.Background(), p)

	if !strings.Contains(item.ErrorText, "sshfleet.transfer.remote_missing: ") {
		t.Fatalf("该报远端路径不存在，实际 %q", item.ErrorText)
	}
	if item.Transfer.SuccessFiles != 0 || item.Transfer.FailFiles != 3 {
		t.Fatalf("一个都没写，四个数应全落在失败上，实际 %+v", item.Transfer)
	}
	if left := remoteRun(t, node, "test -e "+missing); left.ExitCode == nil || *left.ExitCode == 0 {
		t.Fatalf("上传不该顺手把目标目录建出来：%+v", left)
	}
}
