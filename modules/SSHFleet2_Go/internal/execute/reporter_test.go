package execute

// 结果行的三个去处：屏幕、`terminal-output.txt`、执行期日志拿到的是**同一段字**。
//
// 屏幕那一路不打桩（`console` 是唯一出口，只收 `message` 造得出来的话，没有注入口），
// 所以这里比的是文件与日志两处——两处与拼出来的那段字逐字相同，屏幕读的是同一个值。

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"sshfleet2/internal/contract"
	"sshfleet2/internal/message"
)

// TestReporterWritesThreePlaces 一条结果：终端输出文件整段、执行期日志一条，两处逐字相同。
func TestReporterWritesThreePlaces(t *testing.T) {
	dir := t.TempDir()
	term := filepath.Join(dir, "terminal-output.txt")
	execLog := filepath.Join(dir, "SSHFleetExec.log")

	rep, err := newReporter(term, execLog, contract.ModeCommand)
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
	rep.result(res)

	want := message.ResultLine(res, contract.ModeCommand).Text()

	// 一、终端输出文件：整段照抄，连末尾那个换行一起。
	if got := readAll(t, term); got != want {
		t.Fatalf("terminal-output.txt 与拼出来的那段字不一样。\n期望：\n%s\n实际：\n%s", want, got)
	}

	// 二、执行期日志：同一条，前缀是「时间戳  -  [级别]  -  」。
	const prefix = "  -  [INFO   ]  -  "
	body := strings.TrimSuffix(textAfter(t, execLog, prefix), "\n")
	if body != strings.TrimSuffix(want, "\n") {
		t.Fatalf("执行期日志那条与那一行不一样。\n期望：\n%s\n实际：\n%s",
			strings.TrimSuffix(want, "\n"), body)
	}
}

// TestReporterAppendsPerResult 来一条写一条：两条结果就是文件里两段、日志里两条，不攒不覆盖。
func TestReporterAppendsPerResult(t *testing.T) {
	dir := t.TempDir()
	term := filepath.Join(dir, "terminal-output.txt")
	execLog := filepath.Join(dir, "SSHFleetExec.log")

	rep, err := newReporter(term, execLog, contract.ModeCommand)
	if err != nil {
		t.Fatalf("开落点失败：%v", err)
	}
	defer rep.close()

	first := contract.NodeResult{IP: "10.0.0.1", Connected: true, Category: "执行成功"}
	second := contract.NodeResult{IP: "10.0.0.2", Category: "连接失败", ErrorText: "sshfleet.connect.refused: refused"}
	rep.result(first)
	rep.result(second)

	got := readAll(t, term)
	// 两段各自以分隔线收尾：分隔线出现两次，先后顺序就是出行的顺序。
	if n := strings.Count(got, strings.Repeat("=", 50)); n != 2 {
		t.Fatalf("终端输出文件里应有两条结果，分隔线实际出现 %d 次：\n%s", n, got)
	}
	if strings.Index(got, "10.0.0.1") > strings.Index(got, "10.0.0.2") {
		t.Fatalf("两条的先后不对（应是谁先跑完谁在前）：\n%s", got)
	}
	if n := strings.Count(readAll(t, execLog), "  -  [INFO   ]  -  "); n != 2 {
		t.Fatalf("执行期日志应有两条，实际 %d 条", n)
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

// textAfter 取「前缀之后那一整段」，prefix 必须在文件里出现且只出现一次。
func textAfter(t *testing.T, path, prefix string) string {
	t.Helper()
	s := readAll(t, path)
	idx := strings.Index(s, prefix)
	if idx < 0 {
		t.Fatalf("%s 里没有 %q：\n%s", path, prefix, s)
	}
	return s[idx+len(prefix):]
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
// 屏幕的出口是 `console`（它写 `os.Stdout`），测试里把那根管道换掉，抓到的就是使用者
// 看到的那一段；再与 `terminal-output.txt` 逐字比。这条钉的是「版面不许两处各排一遍」。
func TestReporterScreenAndFileMatch(t *testing.T) {
	dir := t.TempDir()
	term := filepath.Join(dir, "terminal-output.txt")
	execLog := filepath.Join(dir, "SSHFleetExec.log")

	rep, err := newReporter(term, execLog, contract.ModeCommand)
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

	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("换管道失败：%v", err)
	}
	os.Stdout = w
	rep.result(res)
	_ = w.Close()
	os.Stdout = old
	rep.close()

	screen, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("读屏幕那一路失败：%v", err)
	}
	if got, want := string(screen), readAll(t, term); got != want {
		t.Fatalf("屏幕与 terminal-output.txt 版面不一样。\n屏幕：\n%s\n文件：\n%s", got, want)
	}
}
