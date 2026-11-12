package guard

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sshfleet2/internal/contract"
	"sshfleet2/internal/execute"
)

// 红线：一台跑完之后，那一行真的从执行站出去了，而且两个落点拿到的是同一段字。
//
// 只判形状：落点有没有内容、同一个 IP 在两处都在、分隔线各出现一次。措辞与字段一个都不判
// ——那是 `message` 的自定义区与 A/B 比对的事。
//
// 「结果行的类型是 console 收的那种」由编译器钉着：`console.Print` 只收 `message.Line`，
// 那个类型的字段不导出，别的包连造都造不出来。所以这条测试判的是另一半——那一段字确实
// 落到了盘上（终端那一路与它读的是同一个值，由执行站一处拼出）。
func TestRedlineExecuteWritesResultLine(t *testing.T) {
	dir := t.TempDir()
	term := filepath.Join(dir, "terminal-output.txt")
	execLog := filepath.Join(dir, "SSHFleetExec.log")

	_, fail := execute.Run(context.Background(), &contract.Plan{
		Mode: contract.ModeCommand,
		Task: contract.Task{Command: &contract.CommandTask{Text: "uptime"}},
		// 连不上也是「跑完一台」：那一行照样要出去，否则失败的那批在盘上一个字都不留。
		Nodes: []contract.Node{{IP: "127.0.0.1", Port: closedPort(t), User: "u", Password: "p"}},
	}, term, execLog)
	if fail != nil {
		t.Fatalf("不该起不来：%+v", fail)
	}

	gone := readText(t, term)
	if !strings.Contains(gone, "127.0.0.1") {
		t.Fatalf("终端输出文件里没有这一台：\n%s", gone)
	}
	// 分隔线只判「有几条」，不判多宽（ADR-0050）：宽度是 `message` 的版式，归它的自定义区
	// 与 A/B 比对；红线要的是「一台跑完只出一条结果」这个形状。
	if n := dividerLines(gone); n != 1 {
		t.Fatalf("终端输出文件里应正好一条结果，分隔线实际 %d 条：\n%s", n, gone)
	}

	logged := readText(t, execLog)
	if !strings.Contains(logged, "127.0.0.1") {
		t.Fatalf("执行期日志里没有这一台：\n%s", logged)
	}
}

// readText 读整份文件。
func readText(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读 %s 失败：%v", path, err)
	}
	return string(b)
}

// dividerLines 数「整行都由 `=` 组成」的那种行有几条。
//
// 不判宽度、不判字符数：宽度是版式，改一次就红的话这条红线就成了版式的哨兵（ADR-0050）。
func dividerLines(text string) int {
	n := 0
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimRight(line, "\r")
		if line != "" && strings.Trim(line, "=") == "" {
			n++
		}
	}
	return n
}
