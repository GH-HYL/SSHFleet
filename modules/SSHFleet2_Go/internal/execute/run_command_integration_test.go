//go:build integration

package execute

// 对着真机器跑命令：正常退出、非 0 退出、命令不存在、超时、被取消、环境变量真的导进去了。
//
// 变量不全就整批 skip（约定同连接那组：SSHFLEET_TEST_HOST / PORT / USER / PASSWORD）。

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"sshfleet2/internal/contract"
	"sshfleet2/internal/message"
)

// TestRunCommandSucceeds 干净退出：退出码 0、输出是命令的输出、没有报错原文。
func TestRunCommandSucceeds(t *testing.T) {
	node, _ := integrationTarget(t)
	p := oneNodePlan(node)
	p.Task.Command.Text = "echo fleet-ok"

	item := runOne(t, context.Background(), p)
	if !item.Connected {
		t.Fatalf("应能连上，错误原文 %q", item.ErrorText)
	}
	if item.ExitCode == nil || *item.ExitCode != 0 {
		t.Fatalf("退出码应是 0，实际 %v", item.ExitCode)
	}
	if item.Output != "fleet-ok" {
		t.Fatalf("输出应是命令那一行，实际 %q", item.Output)
	}
	if item.ErrorText != "" {
		t.Fatalf("干净成功时不该有报错原文，实际 %q", item.ErrorText)
	}
	if item.Cost <= 0 {
		t.Fatalf("执行耗时没记：%v", item.Cost)
	}
	if item.TimedOut || item.Cancelled {
		t.Fatalf("干净成功却带着超时/取消标志：%+v", item)
	}
	// 分类也要真的定出来：干净走完就是本模式成功那一类，判据表一条都不用配。
	if item.Category != "执行成功" {
		t.Fatalf("干净成功该归「执行成功」，实际 %q", item.Category)
	}
}

// TestRunCommandNonZeroExit 非 0 退出：退出码照实给，报错原文仍空——
// 「这一台算不算失败」由结算站按判据表说，执行站不下结论。
func TestRunCommandNonZeroExit(t *testing.T) {
	node, _ := integrationTarget(t)
	p := oneNodePlan(node)
	p.Task.Command.Text = "exit 3"

	item := runOne(t, context.Background(), p)
	if item.ExitCode == nil || *item.ExitCode != 3 {
		t.Fatalf("退出码应是 3，实际 %v", item.ExitCode)
	}
	if item.ErrorText != "" {
		t.Fatalf("非 0 退出不是工具报的错，原文应空，实际 %q", item.ErrorText)
	}
	if item.TimedOut || item.Cancelled {
		t.Fatalf("非 0 退出不该带超时/取消标志：%+v", item)
	}
	// 判据表空着时按退出码兜底：分类名是「执行失败(退出码3)」，退出码本身另有一格。
	if item.Category != "执行失败(退出码3)" {
		t.Fatalf("判据表未命中该按退出码兜底，实际分类 %q", item.Category)
	}
}

// TestRunCommandNotFound 命令不存在：远端解释器自己报 127，与「连不上」「会话被拒」都不是一回事。
func TestRunCommandNotFound(t *testing.T) {
	node, _ := integrationTarget(t)
	p := oneNodePlan(node)
	p.Task.Command.Text = "definitely-not-a-command-xyz"

	item := runOne(t, context.Background(), p)
	if item.ExitCode == nil || *item.ExitCode != 127 {
		t.Fatalf("退出码应是 127，实际 %v（原文 %q）", item.ExitCode, item.ErrorText)
	}
	if !strings.Contains(item.Output, "not found") {
		t.Fatalf("解释器那句「找不到命令」应进输出，实际 %q", item.Output)
	}
}

// TestRunCommandTimesOut 长命令超时：超时是事实（标志 + 报文 + 没有退出码），不是结论。
func TestRunCommandTimesOut(t *testing.T) {
	node, _ := integrationTarget(t)
	p := oneNodePlan(node)
	p.Task.Command.Text = "sleep 10"
	p.Run.TimeoutExecute = 1

	start := time.Now()
	item := runOne(t, context.Background(), p)
	if elapsed := time.Since(start); elapsed > 8*time.Second {
		t.Fatalf("超时没有到点收场，跑了 %v", elapsed)
	}
	if !item.TimedOut {
		t.Fatalf("应记「超时」这个事实，实际 %+v", item)
	}
	if item.Cancelled {
		t.Fatalf("超时被记成了取消：%+v", item)
	}
	if item.ExitCode != nil {
		t.Fatalf("超时没有退出码可给，实际 %v", *item.ExitCode)
	}
	if !strings.HasPrefix(item.ErrorText, "sshfleet.run.timeout: ") {
		t.Fatalf("错误原文应是「命令执行超时」报文，实际 %q", item.ErrorText)
	}
}

// TestRunCommandCancelled 跑的过程中被喊停：那一格是「取消」，不是「超时」。
func TestRunCommandCancelled(t *testing.T) {
	node, _ := integrationTarget(t)
	p := oneNodePlan(node)
	p.Task.Command.Text = "sleep 30"

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(500 * time.Millisecond)
		cancel()
	}()

	start := time.Now()
	item := runOne(t, ctx, p)
	if elapsed := time.Since(start); elapsed > 10*time.Second {
		t.Fatalf("取消之后没有立刻收手，跑了 %v", elapsed)
	}
	if !item.Cancelled {
		t.Fatalf("应记「取消」这个事实，实际 %+v", item)
	}
	if item.TimedOut {
		t.Fatalf("取消被记成了超时：%+v", item)
	}
}

// TestRunCommandAppliesEnv `[env]` 真的导进目标机了：命令读得到它。
//
// 这条是下发行形状的唯一硬证据——拼错一个空格、漏掉 export 段，单测断言的是串，
// 这里断言的是「远端真看得见」。
func TestRunCommandAppliesEnv(t *testing.T) {
	node, _ := integrationTarget(t)
	p := oneNodePlan(node)
	p.Run.Env = map[string]string{"FLEET_PROBE": "ok"}
	p.Task.Command.Text = "echo $FLEET_PROBE"

	item := runOne(t, context.Background(), p)
	if item.ExitCode == nil || *item.ExitCode != 0 {
		t.Fatalf("退出码应是 0，实际 %v（原文 %q）", item.ExitCode, item.ErrorText)
	}
	if item.Output != "ok" {
		t.Fatalf("环境变量没导进去，输出 %q", item.Output)
	}
}

// runOne 跑一台，断言恰好一条事实。
func runOne(t *testing.T, ctx context.Context, p *contract.Plan) contract.NodeResult {
	t.Helper()
	item, _, _ := runOneInto(t, ctx, p)
	return item
}

// runOneInto 跑一台，把这次的两个落点一并交回（要读盘的那几条用）。
func runOneInto(t *testing.T, ctx context.Context, p *contract.Plan) (contract.NodeResult, string, string) {
	t.Helper()
	dir := t.TempDir()
	term := filepath.Join(dir, "terminal-output.txt")
	execLog := filepath.Join(dir, "SSHFleetExec.log")
	paper, fail := Run(ctx, p, term, execLog)
	if fail != nil {
		t.Fatalf("不该起不来：%+v", fail)
	}
	return onlyItem(t, paper), term, execLog
}

// TestRunCommandWritesResultLine 真机跑一台：屏幕上那一行，同时落进终端输出文件与执行期日志。
//
// 三处同一段字：文件里是整段（带末尾换行），日志里是同一个正文加一条时间戳前缀。
func TestRunCommandWritesResultLine(t *testing.T) {
	node, _ := integrationTarget(t)
	p := oneNodePlan(node)
	p.Task.Command.Text = "echo fleet-ok"

	item, term, execLog := runOneInto(t, context.Background(), p)
	if item.Category != "执行成功" {
		t.Fatalf("干净成功该归「执行成功」，实际 %q", item.Category)
	}

	want := message.ResultLine(item, contract.ModeCommand).Text()
	if got := readAll(t, term); got != want {
		t.Fatalf("terminal-output.txt 与那一行不一样。\n期望：\n%s\n实际：\n%s", want, got)
	}

	const prefix = "  -  [INFO   ]  -  "
	body := strings.TrimSuffix(textAfter(t, execLog, prefix), "\n")
	if body != strings.TrimSuffix(want, "\n") {
		t.Fatalf("执行期日志那条与那一行不一样。\n期望：\n%s\n实际：\n%s",
			strings.TrimSuffix(want, "\n"), body)
	}
}
