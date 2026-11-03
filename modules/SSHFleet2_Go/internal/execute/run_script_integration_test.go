//go:build integration

package execute

// 对着真机器跑脚本（10 号单）：正文真的经 stdin 下去、$0 是脚本自己的名字、退出码与输出收得回来。
//
// 脚本模式与命令模式在下发行这一层只差两处——正文走 stdin、shell 家族补 `$0`。这两处
// 单测断言的是串，这里断言的是「远端真按这个名字跑起来」。

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sshfleet2/internal/contract"
)

// TestRunScriptCarriesBodyViaStdin 脚本正文不在命令行里，$0 是脚本文件名。
//
// 断言三样：`$0` 对、正文确实跑到了、命令行里没有正文的一段（正文走的是 stdin）。
func TestRunScriptCarriesBodyViaStdin(t *testing.T) {
	node, _ := integrationTarget(t)
	script := writeScript(t, "probe-name.sh", strings.Join([]string{
		`echo "zero=$0"`,
		`echo "argc=$#"`,
		`echo "body-marker=script-body-ok"`,
	}, "\n")+"\n")

	p := planFor(t, []contract.Node{node}, "-s", script)

	// 正文与命令行分家：命令行里只该有外壳与解释器，正文字节一个都不该出现。
	if strings.Contains(p.Run.Downlink, "script-body-ok") {
		t.Fatalf("正文混进了命令行：%s", p.Run.Downlink)
	}
	if !strings.Contains(p.Run.Stdin, "script-body-ok") {
		t.Fatalf("正文没进 stdin：%q", p.Run.Stdin)
	}

	item := runOne(t, context.Background(), p)
	if !item.Connected {
		t.Fatalf("应能连上，错误原文 %q", item.ErrorText)
	}
	if item.ExitCode == nil || *item.ExitCode != 0 {
		t.Fatalf("退出码应是 0，实际 %v（输出 %q，原文 %q）", item.ExitCode, item.Output, item.ErrorText)
	}
	if want := "zero=" + filepath.Base(script); !strings.Contains(item.Output, want) {
		t.Fatalf("$0 应是脚本文件名 %q，实际输出：\n%s", want, item.Output)
	}
	if !strings.Contains(item.Output, "body-marker=script-body-ok") {
		t.Fatalf("正文没跑到，输出：\n%s", item.Output)
	}
	if !strings.Contains(item.Output, "argc=0") {
		t.Fatalf("脚本没带额外参数，$# 应是 0，输出：\n%s", item.Output)
	}
	if item.Category != "执行成功" {
		t.Fatalf("干净跑完该归「执行成功」，实际 %q", item.Category)
	}
}

// TestRunScriptNonZeroExit 脚本自己以非 0 收场：退出码照实给，与命令模式同口径。
func TestRunScriptNonZeroExit(t *testing.T) {
	node, _ := integrationTarget(t)
	script := writeScript(t, "fail.sh", "echo before-exit\nexit 7\n")

	p := planFor(t, []contract.Node{node}, "-s", script)
	item := runOne(t, context.Background(), p)

	if item.ExitCode == nil || *item.ExitCode != 7 {
		t.Fatalf("退出码应是 7，实际 %v（原文 %q）", item.ExitCode, item.ErrorText)
	}
	if !strings.Contains(item.Output, "before-exit") {
		t.Fatalf("退出前的输出该收回来，实际 %q", item.Output)
	}
	if item.ErrorText != "" {
		t.Fatalf("非 0 退出不是工具报的错，原文应空，实际 %q", item.ErrorText)
	}
}

// TestRunScriptBodyWithSingleQuote 正文里的单引号不该把下发行拼坏。
//
// 正文走 stdin 正是为了这件事，但外壳仍要 `sh -c '…'` 转义一次——这条验那一层没漏。
func TestRunScriptBodyWithSingleQuote(t *testing.T) {
	node, _ := integrationTarget(t)
	script := writeScript(t, "quote.sh", "echo \"it's fine\"\n")

	p := planFor(t, []contract.Node{node}, "-s", script)
	item := runOne(t, context.Background(), p)

	if item.ExitCode == nil || *item.ExitCode != 0 {
		t.Fatalf("退出码应是 0，实际 %v（输出 %q，原文 %q）", item.ExitCode, item.Output, item.ErrorText)
	}
	if !strings.Contains(item.Output, "it's fine") {
		t.Fatalf("带单引号的正文没跑对，输出 %q", item.Output)
	}
}

// writeScript 在临时目录里造一个脚本文件，交回绝对路径。
//
// 绝对路径是要紧的：`planFor` 会把工作目录切到临时配置目录，相对路径那时就指到别处了。
func writeScript(t *testing.T, name, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("造脚本失败：%v", err)
	}
	return path
}
