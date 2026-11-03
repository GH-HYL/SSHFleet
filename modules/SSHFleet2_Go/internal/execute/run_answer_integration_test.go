//go:build integration

package execute

// 对着真机器跑代填（11 号单）：分配 PTY、正文 base64 进命令行、会话 stdin 整条让给代填。
//
// 三条要验的：两问都答上、只配一条时另一条在结果里点名「未送出」、中止词命中即停。

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"sshfleet2/internal/contract"
)

// askTwice 是一个会问两遍的脚本：先问 Q1，再问 Q2，各自回一句。
//
// 用 `printf` + `read` 而不是 `read -p`：提示串该出现在输出里，由代填去认，不靠 shell 的
// 输入提示从哪个流出来。
const askTwice = `printf 'Q1: '
read a
echo "A1=$a"
printf 'Q2: '
read b
echo "A2=$b"
echo "done"
`

// TestRunAnswerFillsBoth 两问都答上：两条都送出去了，输出里能看到填进去的值。
func TestRunAnswerFillsBoth(t *testing.T) {
	node, _ := integrationTarget(t)
	script := writeScript(t, "ask2.sh", askTwice)
	answers := writeAnswers(t, "answers.csv", "first-answer,Q1:\nsecond-answer,Q2:\n")

	p := planFor(t, []contract.Node{node}, "-s", script, "-a", answers)
	if p.Run.Stdin != "" {
		t.Fatalf("代填时正文该编进命令行，stdin 一格应是空的，实际 %q", p.Run.Stdin)
	}

	item := runOne(t, context.Background(), p)
	if len(item.Answer.Missed) != 0 {
		t.Fatalf("两条都该送出去，未送出的是 %v（输出：%s）", item.Answer.Missed, item.Output)
	}
	if item.Answer.AbortLine != "" {
		t.Fatalf("没配中止词却记了中止行 %q", item.Answer.AbortLine)
	}
	if item.TimedOut {
		t.Fatalf("两问都答上了不该超时（输出：%s）", item.Output)
	}
	if item.ExitCode == nil || *item.ExitCode != 0 {
		t.Fatalf("退出码应是 0，实际 %v（输出：%s，原文：%s）", item.ExitCode, item.Output, item.ErrorText)
	}
	for _, want := range []string{"A1=first-answer", "A2=second-answer"} {
		if !strings.Contains(item.Output, want) {
			t.Fatalf("输出里没有 %q：\n%s", want, item.Output)
		}
	}
}

// TestRunAnswerPointsAtMissed 配了两条、只用到一条：没送出去的那条按序号点名。
//
// 「未送出」记的是**表里的条目**（契约 `AnswerResult.Missed`：与 `Answers` 顺序对位），
// 不是没被答上的提问——后者工具认不出来（它不知道那个提问本该有答案）。所以这一条
// 让脚本只问一句、表里放两条：退出码照旧 0，未送出的第 2 条仍要点出来。
func TestRunAnswerPointsAtMissed(t *testing.T) {
	node, _ := integrationTarget(t)
	script := writeScript(t, "ask1.sh", "printf 'Q1: '\nread a\necho \"A1=$a\"\n")
	answers := writeAnswers(t, "answers.csv", "used-answer,Q1:\nnever-used,Q9:\n")

	p := planFor(t, []contract.Node{node}, "-s", script, "-a", answers)
	item := runOne(t, context.Background(), p)

	if item.ExitCode == nil || *item.ExitCode != 0 {
		t.Fatalf("脚本自己干净收场，退出码应是 0，实际 %v（原文 %q）", item.ExitCode, item.ErrorText)
	}
	if len(item.Answer.Missed) != 1 || item.Answer.Missed[0] != 2 {
		t.Fatalf("未送出的应是第 2 条，实际 %v（输出：%s）", item.Answer.Missed, item.Output)
	}
	if !strings.Contains(item.ErrorText, "sshfleet.answer.missed: ") {
		t.Fatalf("报错原文里应有「未送出」那条报文，实际 %q", item.ErrorText)
	}
	if !strings.Contains(item.Output, "A1=used-answer") {
		t.Fatalf("用得上的那条该正常填进去，输出：\n%s", item.Output)
	}
}

// TestRunAnswerExhaustedWhileWaiting 表已送空、远端还在问：记「表已空仍超时」，不冒认「未送出」。
//
// 与上一条是一对：那条是「表里还有条目没匹配上」，这条是「表空了、提问还在」。
// 工具分不出「脚本卡在没配触发词的提示上」与「任务本身耗时长」，只记确定的那件事。
func TestRunAnswerExhaustedWhileWaiting(t *testing.T) {
	node, _ := integrationTarget(t)
	script := writeScript(t, "ask2.sh", askTwice)
	answers := writeAnswers(t, "answers.csv", "only-one,Q1:\n")

	p := planForTweaked(t, []contract.Node{node}, setExecuteTimeout(3), "-s", script, "-a", answers)
	item := runOne(t, context.Background(), p)

	if len(item.Answer.Missed) != 0 {
		t.Fatalf("条目只用了一条、它也送出去了，不该记「未送出」，实际 %v", item.Answer.Missed)
	}
	if !item.Answer.Exhausted {
		t.Fatalf("表空了还在等，应记「表已空仍超时」（输出：%s）", item.Output)
	}
	if !item.TimedOut {
		t.Fatalf("第二问一直没人答，应到点收场（输出：%s）", item.Output)
	}
	if !strings.Contains(item.ErrorText, "sshfleet.answer.exhausted_timeout: ") {
		t.Fatalf("报错原文应是「表已空仍超时」那条报文，实际 %q", item.ErrorText)
	}
	if !strings.Contains(item.Output, "A1=only-one") {
		t.Fatalf("第一条该正常填进去，输出：\n%s", item.Output)
	}
}

// TestRunAnswerStopsOnAbortKeyword 命中中止词立刻关会话：不再往下填，原文进结果。
//
// 中止词取判据表里「密码过期」那一类——继续把密码往一个不肯收的地方灌没有意义。
func TestRunAnswerStopsOnAbortKeyword(t *testing.T) {
	node, _ := integrationTarget(t)
	script := writeScript(t, "expired.sh", `echo 'WARNING: Your password has expired.'
sleep 30
`)
	answers := writeAnswers(t, "answers.csv", "never-sent,Q1:\n")

	p := planForTweaked(t, []contract.Node{node}, setExecuteTimeout(20), "-s", script, "-a", answers)
	item := runOne(t, context.Background(), p)

	if item.Answer.AbortLine == "" {
		t.Fatalf("命中中止词却没记原文（输出：%s）", item.Output)
	}
	if !strings.Contains(item.Answer.AbortLine, "Your password has expired") {
		t.Fatalf("中止行应是命中那一行的原文，实际 %q", item.Answer.AbortLine)
	}
	if item.TimedOut {
		t.Fatalf("命中中止词该立刻关会话，不该等到超时（耗时 %v）", item.Cost)
	}
	if item.Cost > 10*time.Second {
		t.Fatalf("关会话太慢：%v", item.Cost)
	}
}

// TestRunAnswerFollowsMatchOptions 匹配口径从纸上来（配置 `[interactive] regex`）。
//
// 触发词写成 `Q[0-9]:`：按子串匹配永远对不上，按正则才命中。这条要是绿了，说明
// 执行站认的是纸上那两个开关，不是自己读配置、也不是写死子串。
func TestRunAnswerFollowsMatchOptions(t *testing.T) {
	node, _ := integrationTarget(t)
	script := writeScript(t, "ask1.sh", "printf 'Q1: '\nread a\necho \"A1=$a\"\n")
	answers := writeAnswers(t, "answers.csv", "regex-answer,Q[0-9]:\n")

	p := planForTweaked(t, []contract.Node{node}, func(body string) string {
		return strings.Replace(body, "regex          = false", "regex          = true", 1)
	}, "-s", script, "-a", answers)

	item := runOne(t, context.Background(), p)
	if len(item.Answer.Missed) != 0 {
		t.Fatalf("正则口径下该命中，实际未送出 %v（输出：%s）", item.Answer.Missed, item.Output)
	}
	if !strings.Contains(item.Output, "A1=regex-answer") {
		t.Fatalf("正则触发词没填上，输出：\n%s", item.Output)
	}
}

// writeAnswers 造一份代填表文件（第一列内容、第二列起触发词），交回绝对路径。
func writeAnswers(t *testing.T, name, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("造代填表失败：%v", err)
	}
	return path
}
