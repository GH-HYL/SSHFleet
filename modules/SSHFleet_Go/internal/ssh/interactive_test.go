package ssh

import (
	"encoding/base64"
	"strings"
	"testing"
)

// 采集侧整备与下发行形态的测试：这两处出错是静默的（\r 残留只是显示噪声，
// 下发行拼错要到真机上才炸），必须钉住。

// 行尾 \r 在采集侧去掉：PTY 的 onlcr 把远端的 \n 输出成 \r\n。
func TestOutputHookStripsLineEndCR(t *testing.T) {
	out := &lockedBuffer{}
	h := newOutputHook(out, InteractiveInput{Answers: []Answer{{Value: "1", Triggers: []string{"架构"}}}})

	_, _ = h.Write([]byte("请选择架构\r\n"))
	if got := out.String(); got != "请选择架构\n" {
		t.Fatalf("行尾 \\r 应被去掉，实际 %q", got)
	}

	ev := <-h.events
	if ev.kind != eventSend || ev.text != "1" {
		t.Fatalf("命中应发出「送这条内容」事件，实际 %+v", ev)
	}
}

// \r\n 被读边界切开（\r 在上一块末尾、\n 在下一块开头）时也要去掉。
func TestOutputHookStripsCRSplitAcrossChunks(t *testing.T) {
	out := &lockedBuffer{}
	h := newOutputHook(out, InteractiveInput{})

	_, _ = h.Write([]byte("第一行\r"))
	if got := out.String(); got != "第一行" {
		t.Fatalf("待定的 \\r 不该先落缓冲，实际 %q", got)
	}
	_, _ = h.Write([]byte("\n第二行\r\n"))
	if got := out.String(); got != "第一行\n第二行\n" {
		t.Fatalf("跨块的 \\r\\n 也该去掉，实际 %q", got)
	}
}

// 不是行尾的 \r（进度条原地重写）保留原样。
func TestOutputHookKeepsBareCR(t *testing.T) {
	out := &lockedBuffer{}
	h := newOutputHook(out, InteractiveInput{})

	_, _ = h.Write([]byte("10%\r20%"))
	if got := out.String(); got != "10%\r20%" {
		t.Fatalf("非行尾的 \\r 应保留，实际 %q", got)
	}
	h.Flush()
	if got := out.String(); got != "10%\r20%" {
		t.Fatalf("没有待定字节时 Flush 不该改动缓冲，实际 %q", got)
	}
}

// 中止词优先：同一块里既有触发词又有中止词时，只发中止信号、不再送代填。
func TestOutputHookAbortWins(t *testing.T) {
	out := &lockedBuffer{}
	h := newOutputHook(out, InteractiveInput{
		Answers:       []Answer{{Value: "1", Triggers: []string{"架构"}}},
		AbortKeywords: []string{"password has expired"},
	})

	_, _ = h.Write([]byte("WARNING: Your password has expired.\n请选择架构"))
	ev := <-h.events
	if ev.kind != eventAbort || !strings.Contains(ev.text, "password has expired") {
		t.Fatalf("应发出中止事件且带命中原文行，实际 %+v", ev)
	}
	select {
	case extra := <-h.events:
		t.Fatalf("中止后不该再有事件，实际 %+v", extra)
	default:
	}
}

// （原 TestMissText 已随「未命中不再拼文案」一起删除：未命中现在是结构事实
// AnswersMissed，成败与分类由结果判定产生，执行侧不再生成结论文案。）

// 下发行形态：正文 base64 编入命令行、作内层解释器的 -c 参数，最外层只有一层单引号。
func TestInteractiveCommandForm(t *testing.T) {
	enc := base64.StdEncoding.EncodeToString([]byte("who -b"))
	got := InteractiveCommand(InteractiveInput{Command: "who -b", Interpreter: "bash"})
	want := "sh -c 'export LC_ALL=en_US.UTF-8 LANG=en_US.UTF-8 PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin; bash -c \"$(printf %s " + enc + " | base64 -d)\"'"
	if got != want {
		t.Fatalf("下发行形态不对\n实际：%s\n应为：%s", got, want)
	}
}

// sudo 时提权的是解释器本身；脚本模式按解释器换 python3；正文不进命令行原文。
func TestInteractiveCommandInterpreterAndRoot(t *testing.T) {
	if root := InteractiveCommand(InteractiveInput{Command: "id", AsRoot: true, Interpreter: "bash"}); !strings.Contains(root, "sudo bash -c") {
		t.Fatalf("sudo 应提权内层解释器：%s", root)
	}
	py := InteractiveCommand(InteractiveInput{ScriptBody: "print(1)", Interpreter: "python3"})
	if !strings.Contains(py, "python3 -c") {
		t.Fatalf("脚本模式应换 python3 解释：%s", py)
	}
	if strings.Contains(py, "print(1)") {
		t.Fatalf("正文不该以原文出现在命令行里：%s", py)
	}
}

// -a 的脚本模式：正文照旧 base64 进命令行，脚本名作 `-c` 之后的第一个参数——那正是
// bash 的 $0，脚本据此仍能从自己的文件名里取信息。
func TestInteractiveCommandKeepsScriptName(t *testing.T) {
	got := InteractiveCommand(InteractiveInput{ScriptBody: "echo hi", ScriptName: "(1.2.3.4).sh", Interpreter: "bash"})
	idx := strings.Index(got, "base64 -d)")
	if idx < 0 {
		t.Fatalf("下发行形态变了：%s", got)
	}
	if !strings.Contains(got[idx:], "(1.2.3.4).sh") {
		t.Fatalf("脚本名应挂在 -c 之后当 $0，实际：%s", got)
	}
}

// 命令模式（-c）没有脚本文件，不该凭空补一个名字。
func TestInteractiveCommandNoScriptNameForCommand(t *testing.T) {
	got := InteractiveCommand(InteractiveInput{Command: "who -b", ScriptName: "t.sh", Interpreter: "bash"})
	if strings.Contains(got, "t.sh") {
		t.Fatalf("命令模式不该出现脚本名，实际：%s", got)
	}
}

// 日志说明与方向稿的形态一致：原始内容 / 处理方式 / 最终命令三段。
func TestDescribeInteractive(t *testing.T) {
	text := DescribeInteractive(DescribeInteractiveInput{
		Command: "who -b", Answers: 2, Downlink: "sh -c 'x'",
	})
	for _, want := range []string{"原始命令： who -b", "会话 stdin 整条让给代填（2 条）", "最终命令： sh -c 'x'"} {
		if !strings.Contains(text, want) {
			t.Fatalf("日志说明缺少 %q：\n%s", want, text)
		}
	}

	text = DescribeInteractive(DescribeInteractiveInput{
		ScriptPath: "deploy.sh", ScriptBody: "echo hi", Answers: 1, Downlink: "sh -c 'y'",
	})
	if !strings.Contains(text, "原始脚本： deploy.sh（正文经 base64 编入命令行，不落盘）") {
		t.Fatalf("脚本模式的日志说明不对：\n%s", text)
	}
}
