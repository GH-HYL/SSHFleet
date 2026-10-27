package batch

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sshfleet/internal/cli"
	"sshfleet/internal/nodelist"
	"sshfleet/internal/ssh"
	"sshfleet/internal/verdict"
)

func testKeywords(t *testing.T) *verdict.Keywords {
	t.Helper()
	path := filepath.Join(t.TempDir(), "kw.toml")
	body := "[[categories]]\nname = '连不上'\nkeywords = ['timeout']\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	kw, err := verdict.LoadKeywords(path)
	if err != nil {
		t.Fatalf("造判据表失败：%v", err)
	}
	return kw
}

// 执行期那三条内部错误：走公开入口 Run，钉住文案与形态。
// 它们不建连——错在建任务这一步，节点信息给一条就够。
func TestRunReportsInternalStateErrors(t *testing.T) {
	kw := testKeywords(t)
	nodes := &nodelist.Nodes{Items: []nodelist.NodeInfo{{IP: "1.1.1.1", Port: 22, User: "root"}}}

	t.Run("上传清单缺失", func(t *testing.T) {
		a := &cli.Args{Upload: "./src", Path: "/tmp/", Number: 1}
		_, err := Run(context.Background(), a, nil, nodes, nil, nil, kw, nil, Hooks{})
		if err == nil {
			t.Fatal("上传模式下没有清单，应报内部错误")
		}
		want := "上传清单缺失\n原因：主干未在上传模式下采集清单\n提示：这是内部错误，请把本次命令反馈给作者"
		if got := err.Error(); got != want {
			t.Fatalf("文案应为 %q，实际：%q", want, got)
		}
	})

	t.Run("读取脚本文件失败", func(t *testing.T) {
		a := &cli.Args{Script: filepath.Join(t.TempDir(), "nope.sh"), Number: 1}
		_, err := Run(context.Background(), a, nil, nodes, nil, nil, kw, nil, Hooks{})
		if err == nil {
			t.Fatal("脚本读不到时应报错")
		}
		if !strings.HasPrefix(err.Error(), "读取脚本文件失败：") {
			t.Fatalf("文案应点明读取脚本失败，实际：%q", err.Error())
		}
		if !strings.Contains(err.Error(), "\n原因：") || strings.Contains(err.Error(), "\n提示：") {
			t.Fatalf("应为两行（问题 + 原因），实际：%q", err.Error())
		}
	})

	t.Run("下发行缺失", func(t *testing.T) {
		a := &cli.Args{
			Interpreter: "bash",
			Answers:     []ssh.Answer{{Value: "y", Triggers: []string{"continue?"}}},
			Number:      1,
			// Downlink 留空：合规检查阶段本该填好，漏了才走到这里
		}
		_, err := Run(context.Background(), a, nil, nodes, nil, nil, kw, nil, Hooks{})
		if err == nil {
			t.Fatal("交互分支缺下发行，应报内部错误")
		}
		want := "没有为这次交互执行拼出下发行\n原因：选项/参数合规检查阶段未产出 Args.Downlink\n提示：这是内部错误，请把本次命令反馈给作者"
		if got := err.Error(); got != want {
			t.Fatalf("文案应为 %q，实际：%q", want, got)
		}
	})
}
