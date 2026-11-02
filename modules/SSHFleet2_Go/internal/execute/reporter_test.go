package execute

// 一条结果的三个去处：屏幕与 `terminal-output.txt` 拿到同一段字，执行期日志写节点明细。
//
// 屏幕那一路的流可注入（`newScreen(w)`），所以这里拿一个 `bytes.Buffer` 当屏幕：进来看字节、
// 进来什么就证明屏幕上是什么。生产里那条流由 `console.Writer()` 给，只此一处。

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"sshfleet2/internal/contract"
	"sshfleet2/internal/message"
)

// TestReporterWritesThreePlaces 一条结果三个去处：屏幕与文件同一段字，日志是节点明细。
func TestReporterWritesThreePlaces(t *testing.T) {
	dir := t.TempDir()
	term := filepath.Join(dir, "terminal-output.txt")
	execLog := filepath.Join(dir, "SSHFleetExec.log")

	var screenBuf bytes.Buffer
	rep, err := newReporter(newScreen(&screenBuf), term, execLog, contract.ModeCommand)
	if err != nil {
		t.Fatalf("开落点失败：%v", err)
	}
	defer rep.close()

	code := 3
	res := contract.NodeResult{
		IP:          "10.0.0.1",
		Connected:   true,
		ConnectCost: 123 * time.Millisecond,
		Cost:        456 * time.Millisecond,
		Category:    "执行失败(退出码3)",
		ExitCode:    &code,
		ErrorText:   "sshfleet.run.rejected: rejected by remote",
		Output:      "line one\nline two",
	}
	rep.result(res, "sshfleet")

	want := message.ResultLine(res, contract.ModeCommand).Text()

	// 一、屏幕与终端输出文件：两处都是那一段字，连末尾那个换行一起。
	if got := readAll(t, term); got != want {
		t.Fatalf("terminal-output.txt 与拼出来的那段字不一样。\n期望：\n%s\n实际：\n%s", want, got)
	}
	if got := screenBuf.String(); got != want {
		t.Fatalf("屏幕与拼出来的那段字不一样。\n期望：\n%s\n实际：\n%s", want, got)
	}

	// 二、执行期日志：**不是**那一段字，而是五级明细（给人看的版面进日志只会把版面拉稀）。
	logged := readAll(t, execLog)
	if strings.Contains(logged, strings.Repeat("=", 50)) {
		t.Fatalf("日志里不该出现给人看的那条分隔线：\n%s", logged)
	}
	for _, want := range []string{
		"【10.0.0.1】连接成功，用户 sshfleet，耗时 0.123s",
		"【10.0.0.1】命令执行失败，退出码 3，耗时 0.456s",
		"【10.0.0.1】错误详情：sshfleet.run.rejected: rejected by remote",
		"【10.0.0.1】分类: 执行失败(退出码3)",
	} {
		if !strings.Contains(logged, want) {
			t.Fatalf("日志里少了这一级：%q\n实际：\n%s", want, logged)
		}
	}
}

// TestReporterAppendsPerResult 来一条写一条：两条结果就是文件里两段、日志里两条，不攒不覆盖。
func TestReporterAppendsPerResult(t *testing.T) {
	dir := t.TempDir()
	term := filepath.Join(dir, "terminal-output.txt")
	execLog := filepath.Join(dir, "SSHFleetExec.log")

	var screenBuf bytes.Buffer
	rep, err := newReporter(newScreen(&screenBuf), term, execLog, contract.ModeCommand)
	if err != nil {
		t.Fatalf("开落点失败：%v", err)
	}
	defer rep.close()

	first := contract.NodeResult{IP: "10.0.0.1", Connected: true, Category: "执行成功"}
	second := contract.NodeResult{IP: "10.0.0.2", Category: "连接失败", ErrorText: "sshfleet.connect.refused: refused"}
	rep.result(first, "sshfleet")
	rep.result(second, "sshfleet")

	got := readAll(t, term)
	// 两段各自以分隔线收尾：分隔线出现两次，先后顺序就是出行的顺序。
	if n := strings.Count(got, strings.Repeat("=", 50)); n != 2 {
		t.Fatalf("终端输出文件里应有两条结果，分隔线实际出现 %d 次：\n%s", n, got)
	}
	if strings.Index(got, "10.0.0.1") > strings.Index(got, "10.0.0.2") {
		t.Fatalf("两条的先后不对（应是谁先跑完谁在前）：\n%s", got)
	}
	// 日志里成功那台只有一行（`成功：…`），失败那台才有「分类」收尾——与老版同一口径。
	logged := readAll(t, execLog)
	if n := strings.Count(logged, "】成功："); n != 1 {
		t.Fatalf("成功那台应正好一行，实际 %d 行：\n%s", n, logged)
	}
	if n := strings.Count(logged, "】分类: "); n != 1 {
		t.Fatalf("失败那台应正好一条分类行，实际 %d 条：\n%s", n, logged)
	}
}

// readAll 读整份文件。
func readAll(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读 %s 失败：%v", path, err)
	}
	return string(b)
}

// TestResultLineKeepsNoSecret 内联清单里的密码不进那两个落点。
//
// 那一行的每一格都是事实：IP、连接成败、耗时、分类、原文、退出码——从来没有凭据位。
// 这条钉的是「以后别往里加一栏」：真跑一台连不上的机器，盘上不该出现那个密码。
func TestResultLineKeepsNoSecret(t *testing.T) {
	const password = "S3cret#2026"
	p := plan("127.0.0.1", deadPort(t))
	p.Nodes[0].Password = password

	dir := t.TempDir()
	term := filepath.Join(dir, "terminal-output.txt")
	execLog := filepath.Join(dir, "SSHFleetExec.log")
	if _, fail := Run(context.Background(), p, term, execLog); fail != nil {
		t.Fatalf("不该起不来：%+v", fail)
	}

	for _, path := range []string{term, execLog} {
		if strings.Contains(readAll(t, path), password) {
			t.Fatalf("%s 里出现了明文密码", path)
		}
	}
}

// TestReporterScreenAndFileMatch 屏幕上打的那一段与落盘的那一段是同一段字：版面只有一版。
//
// 流注入成一个缓冲，抓到的就是使用者看到的那一段；再与 `terminal-output.txt` 逐字比。
// 这条钉的是「版面不许两处各排一遍」。
func TestReporterScreenAndFileMatch(t *testing.T) {
	dir := t.TempDir()
	term := filepath.Join(dir, "terminal-output.txt")
	execLog := filepath.Join(dir, "SSHFleetExec.log")

	var screenBuf bytes.Buffer
	rep, err := newReporter(newScreen(&screenBuf), term, execLog, contract.ModeCommand)
	if err != nil {
		t.Fatalf("开落点失败：%v", err)
	}

	code := 3
	res := contract.NodeResult{
		IP:          "10.0.0.1",
		Connected:   true,
		ConnectCost: 123 * time.Millisecond,
		Cost:        456 * time.Millisecond,
		Category:    "执行失败(退出码3)",
		ExitCode:    &code,
		ErrorText:   "sshfleet.run.rejected: rejected by remote",
		Output:      "line one\nline two",
	}

	rep.result(res, "sshfleet")
	rep.close()

	if got, want := screenBuf.String(), readAll(t, term); got != want {
		t.Fatalf("屏幕与 terminal-output.txt 版面不一样。\n屏幕：\n%s\n文件：\n%s", got, want)
	}
}
