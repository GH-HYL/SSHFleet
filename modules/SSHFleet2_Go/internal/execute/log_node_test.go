package execute

// 执行期日志的节点明细：期望串写死，照老版 `output/reporter.go` 的 `logNode` 抄下来的形状
// （级别、节点前缀、五个级别的先后）。

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"sshfleet2/internal/contract"
	"sshfleet2/internal/log"
)

// captureNodeLog 跑一次 logNode，交回整份日志的正文。
//
// 每次用一份新文件：日志是追加写的，复用会把上一次的算进来。
func captureNodeLog(t *testing.T, res contract.NodeResult, mode contract.Mode) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "SSHFleetExec.log")
	lg, err := log.InitExec(path)
	if err != nil {
		t.Fatalf("开执行期日志失败：%v", err)
	}
	logNode(lg, res, mode, "history/本次/terminal-output.txt", "sshfleet")
	if err := lg.Close(); err != nil {
		t.Fatalf("关执行期日志失败：%v", err)
	}
	return readAll(t, path)
}

// TestLogNodeSuccess 成功那台只写一行：动作名 + 两段耗时，不带分类行。
func TestLogNodeSuccess(t *testing.T) {
	res := contract.NodeResult{
		IP:          "10.0.0.1",
		Connected:   true,
		ConnectCost: 123 * time.Millisecond,
		Cost:        456 * time.Millisecond,
		Category:    "执行成功",
	}
	got := captureNodeLog(t, res, contract.ModeCommand)
	if want := "  -  [SUCCESS]  -  【10.0.0.1】成功：连接 0.123s，执行 0.456s"; !strings.Contains(got, want) {
		t.Fatalf("成功那行不对，缺 %q：\n%s", want, got)
	}
	if strings.Contains(got, "】分类: ") {
		t.Fatalf("成功那台不该有分类行（老版成功只写一行）：\n%s", got)
	}
}

// TestLogNodeConnectFailure 连不上：两级——连接失败（带原文）+ 分类，不写「执行失败」。
func TestLogNodeConnectFailure(t *testing.T) {
	res := contract.NodeResult{
		IP:        "10.0.0.2",
		Category:  "连接失败",
		ErrorText: "sshfleet.connect.refused: dial tcp 10.0.0.2:22: connect: connection refused",
	}
	got := captureNodeLog(t, res, contract.ModeCommand)
	for _, want := range []string{
		"  -  [ERROR  ]  -  【10.0.0.2】连接失败，用户 sshfleet：sshfleet.connect.refused: dial tcp 10.0.0.2:22: connect: connection refused",
		"  -  [INFO   ]  -  【10.0.0.2】分类: 连接失败",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("缺这一级 %q：\n%s", want, got)
		}
	}
	if strings.Contains(got, "命令执行失败") {
		t.Fatalf("没连上，不该有「执行失败」那一级：\n%s", got)
	}
}

// TestLogNodeRunFailure 连上了却失败：五级全齐，级别依次是 SUCCESS / ERROR / ERROR / WARNING / INFO。
func TestLogNodeRunFailure(t *testing.T) {
	code := 3
	res := contract.NodeResult{
		IP:          "10.0.0.1",
		Connected:   true,
		ConnectCost: 123 * time.Millisecond,
		Cost:        456 * time.Millisecond,
		Category:    "执行失败(退出码3)",
		ExitCode:    &code,
		ErrorText:   "sshfleet.run.rejected: rejected by remote",
		Output:      "boom",
	}
	got := captureNodeLog(t, res, contract.ModeCommand)
	want := []string{
		"  -  [SUCCESS]  -  【10.0.0.1】连接成功，用户 sshfleet，耗时 0.123s\n",
		"  -  [ERROR  ]  -  【10.0.0.1】命令执行失败，退出码 3，耗时 0.456s\n",
		"  -  [ERROR  ]  -  【10.0.0.1】错误详情：sshfleet.run.rejected: rejected by remote\n",
		"  -  [WARNING]  -  【10.0.0.1】输出明细（1 行）：\n",
		"  -  [INFO   ]  -  【10.0.0.1】分类: 执行失败(退出码3)\n",
	}
	pos := 0
	for _, w := range want {
		idx := strings.Index(got[pos:], w)
		if idx < 0 {
			t.Fatalf("日志里缺这一级或次序不对：%q\n实际：\n%s", w, got)
		}
		pos += idx + len(w)
	}
	if !strings.Contains(got, "【10.0.0.1】  boom\n") {
		t.Fatalf("输出明细那一行应带节点前缀与两格缩进：\n%s", got)
	}
}

// TestLogNodeSkipsDuplicateOutput 输出明细与错误详情逐字相同：不重复写一遍。
func TestLogNodeSkipsDuplicateOutput(t *testing.T) {
	res := contract.NodeResult{
		IP:        "10.0.0.1",
		Connected: true,
		Category:  "执行失败(退出码1)",
		ErrorText: "Permission denied (publickey,password).",
		Output:    "Permission denied (publickey,password).",
	}
	got := captureNodeLog(t, res, contract.ModeCommand)
	if strings.Contains(got, "输出明细") {
		t.Fatalf("与错误详情相同的输出不该再写一遍：\n%s", got)
	}
}

// TestLogNodeCapsOutput 输出明细封顶：抬头报总行数，只写前 50 行，末尾交代其余与去处。
func TestLogNodeCapsOutput(t *testing.T) {
	lines := make([]string, 0, 60)
	for i := 1; i <= 60; i++ {
		lines = append(lines, fmt.Sprintf("line %d", i))
	}
	res := contract.NodeResult{
		IP:        "10.0.0.1",
		Connected: true,
		Category:  "执行失败(退出码1)",
		Output:    strings.Join(lines, "\n"),
	}
	got := captureNodeLog(t, res, contract.ModeCommand)

	if !strings.Contains(got, "【10.0.0.1】输出明细（60 行）：") {
		t.Fatalf("抬头应报总行数 60：\n%s", got)
	}
	if !strings.Contains(got, "【10.0.0.1】  line 50") {
		t.Fatalf("第 50 行该写出来：\n%s", got)
	}
	if strings.Contains(got, "line 51") {
		t.Fatalf("第 51 行不该写出来（上限 50 行）：\n%s", got)
	}
	if !strings.Contains(got, "……其余 10 行省略（完整内容见 history/本次/terminal-output.txt）") {
		t.Fatalf("末尾应交代省略了多少行、完整内容在哪：\n%s", got)
	}
}

// TestLogNodeTruncatesLongLine 单行超长截断到 500 字加省略号。
func TestLogNodeTruncatesLongLine(t *testing.T) {
	long := strings.Repeat("x", 600)
	res := contract.NodeResult{
		IP:        "10.0.0.1",
		Connected: true,
		Category:  "执行失败(退出码1)",
		Output:    long,
	}
	got := captureNodeLog(t, res, contract.ModeCommand)
	if strings.Contains(got, long) {
		t.Fatalf("整行 600 字不该原样写进日志：\n%s", got)
	}
	if !strings.Contains(got, strings.Repeat("x", 500)+"…") {
		t.Fatalf("应截到 500 字加省略号：\n%s", got)
	}
}
