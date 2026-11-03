//go:build integration

package execute

// 对着真机器下载（14 号单）：按 IP 建子目录、层级保留、软链接过滤但列出来、中间步骤各自记账。

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sshfleet2/internal/contract"
)

// TestRunDownloadKeepsTreeAndSkipsSymlinks 一棵含软链接的远端目录：真文件按原层级收回，
// 软链接过滤并在明细里点名，两条中间命令的退出码进 Steps。
func TestRunDownloadKeepsTreeAndSkipsSymlinks(t *testing.T) {
	node, _ := integrationTarget(t)
	remote := remoteTempDir(t, node, remoteWritable)

	setup := `mkdir -p ` + remote + `/sub && printf alpha > ` + remote + `/a.txt && printf beta > ` + remote + `/sub/b.txt && ln -s a.txt ` + remote + `/link.txt`
	if item := remoteRun(t, node, setup); item.ExitCode == nil || *item.ExitCode != 0 {
		t.Fatalf("摆现场失败：%+v", item)
	}

	local := t.TempDir()
	p := planFor(t, []contract.Node{node}, "-d", remote, "-p", local)
	item := runOne(t, context.Background(), p)

	if item.ExitCode != nil {
		t.Fatalf("下载没有目的命令，退出码那一格该空着，实际 %v", *item.ExitCode)
	}
	if item.ErrorText != "" {
		t.Fatalf("干净下完不该有报错原文，实际 %q", item.ErrorText)
	}
	if item.Transfer.TotalFiles != 2 || item.Transfer.SuccessFiles != 2 || item.Transfer.FailFiles != 0 {
		t.Fatalf("四个数不对：%+v（明细：%s）", item.Transfer, item.Output)
	}
	if !strings.Contains(item.Output, "link.txt: 已跳过（符号链接）") {
		t.Fatalf("软链接该被点名，明细：\n%s", item.Output)
	}

	// 两条中间命令各一条退出码，且都是 0——它们不进 ExitCode 那一格。
	if len(item.Steps) != 2 {
		t.Fatalf("预检与枚举该各记一条中间步骤，实际 %+v", item.Steps)
	}
	for _, s := range item.Steps {
		if s.ExitCode == nil || *s.ExitCode != 0 {
			t.Fatalf("中间步骤 %s 的退出码不对：%+v", s.Name, item.Steps)
		}
	}

	// 落地位置：本机目标目录下按 IP 建一层，层级照远端。
	ipDir := filepath.Join(local, node.IP)
	for _, probe := range []struct{ rel, want string }{
		{"a.txt", "alpha"},
		{"sub/b.txt", "beta"},
	} {
		data, err := os.ReadFile(filepath.Join(ipDir, probe.rel))
		if err != nil {
			t.Fatalf("%s 没落下来：%v", probe.rel, err)
		}
		if string(data) != probe.want {
			t.Fatalf("%s 的内容不对：%q", probe.rel, string(data))
		}
	}
	if _, err := os.Lstat(filepath.Join(ipDir, "link.txt")); err == nil {
		t.Fatal("软链接被落成了本地文件")
	}
}

// TestRunDownloadMissingPath 远端路径不存在：预检那条的退出码进 Steps，报错说「不存在」。
func TestRunDownloadMissingPath(t *testing.T) {
	node, _ := integrationTarget(t)
	missing := remoteWritable + "/definitely-not-there"

	p := planFor(t, []contract.Node{node}, "-d", missing, "-p", t.TempDir())
	item := runOne(t, context.Background(), p)

	if !strings.Contains(item.ErrorText, "sshfleet.transfer.remote_missing: ") {
		t.Fatalf("该报远端路径不存在，实际 %q", item.ErrorText)
	}
	if len(item.Steps) != 1 || item.Steps[0].Name != contract.StepPrecheck {
		t.Fatalf("预检该记一条中间步骤，实际 %+v", item.Steps)
	}
	if code := item.Steps[0].ExitCode; code == nil || *code == 0 {
		t.Fatalf("预检没通过，那条的退出码不该是 0：%+v", item.Steps)
	}
	if item.ExitCode != nil {
		t.Fatalf("中间步骤的失败不该顶成结果退出码，实际 %v", *item.ExitCode)
	}
}

// TestRunDownloadAllSymlinks 目录里全是软链接：过滤后没有可下载的，措辞说「全是链接」。
func TestRunDownloadAllSymlinks(t *testing.T) {
	node, _ := integrationTarget(t)
	remote := remoteTempDir(t, node, remoteWritable)

	setup := `printf x > ` + remote + `/real.txt && ln -s real.txt ` + remote + `/l1.txt`
	if item := remoteRun(t, node, setup); item.ExitCode == nil || *item.ExitCode != 0 {
		t.Fatalf("摆现场失败：%+v", item)
	}
	// 把真文件藏起来：只剩一条链，让它成为「目录里唯一的东西」。
	if item := remoteRun(t, node, "rm "+remote+"/real.txt"); item.ExitCode == nil || *item.ExitCode != 0 {
		t.Fatalf("删真文件失败：%+v", item)
	}

	p := planFor(t, []contract.Node{node}, "-d", remote, "-p", t.TempDir())
	item := runOne(t, context.Background(), p)

	if !strings.Contains(item.ErrorText, "sshfleet.transfer.no_files: ") {
		t.Fatalf("该报没有可下载的文件，实际 %q", item.ErrorText)
	}
	if !strings.Contains(item.ErrorText, "symlink") {
		t.Fatalf("措辞该说「全是链接」，实际 %q", item.ErrorText)
	}
}
