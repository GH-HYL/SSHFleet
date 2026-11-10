package parse

// 代填表（建站 05 的 05 号单）：三种来源、四种错、正则预编译。
//
// 用例直接调 answerTable：它不读配置、不读清单，参数只有「值 + 两个匹配口径」，
// 所以不必造临时配置、也不必切目录——相对路径的 testdata 因此可用。

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sshfleet2/internal/contract"
	"sshfleet2/internal/message"
)

func TestAnswerTableInline(t *testing.T) {
	answers, fail := answerTable("y,是否继续", false, false)
	if fail != nil {
		t.Fatalf("不该报错：%v", message.CannotStartText(*fail))
	}
	if len(answers) != 1 {
		t.Fatalf("内联一条应当读出一条：%v", answers)
	}
	if answers[0].Value != "y" || len(answers[0].Triggers) != 1 || answers[0].Triggers[0] != "是否继续" {
		t.Fatalf("内容或触发词不对：%+v", answers[0])
	}
}

// 第 1 列留空是合法写法：只发一个回车。
func TestAnswerTableInlineEmptyValue(t *testing.T) {
	answers, fail := answerTable(",是否继续", false, false)
	if fail != nil {
		t.Fatalf("不该报错：%v", message.CannotStartText(*fail))
	}
	if answers[0].Value != "" {
		t.Fatalf("第一列留空应当读成空串：%q", answers[0].Value)
	}
}

// 文件来源：注释行与空行跳过，列数自适应。
func TestAnswerTableFromFile(t *testing.T) {
	answers, fail := answerTable("testdata/answers.csv", false, false)
	if fail != nil {
		t.Fatalf("不该报错：%v", message.CannotStartText(*fail))
	}
	if len(answers) != 2 {
		t.Fatalf("应当读出两条：%+v", answers)
	}
	if len(answers[0].Triggers) != 2 || answers[0].Triggers[1] != "continue" {
		t.Fatalf("第一条的两个触发词不对：%+v", answers[0])
	}
	if answers[1].Value != "" || answers[1].Triggers[0] != "按回车继续" {
		t.Fatalf("第二条不对：%+v", answers[1])
	}
}

// 四种错各有一句能照做的话。
func TestAnswerTableErrors(t *testing.T) {
	cases := []struct {
		name  string
		raw   string
		regex bool
		want  string
	}{
		{"只有一列的文件", "testdata/one_col.csv", true, "缺触发词"},
		{"只有逗号", "y,", true, "触发词是空的"},
		{"一条都没有", "testdata/empty_answers.csv", true, "里没有一条代填"},
		{"文件是二进制", "testdata/answers.bin", true, "是二进制文件"},
		{"路径读不到", "testdata/根本没有这个.csv", true, "读不到"},
	}
	for _, c := range cases {
		_, fail := answerTable(c.raw, c.regex, false)
		if fail == nil {
			t.Errorf("%s：应当报错", c.name)
			continue
		}
		if !strings.Contains(message.CannotStartText(*fail), c.want) {
			t.Errorf("%s：报的话里没有 %q：%s", c.name, c.want, message.CannotStartText(*fail))
		}
	}
}

// 「只有一列」是缺触发词还是缺列：单列、逗号结尾、中间空列都算触发词这一侧的问题。
func TestAnswerTableMissingTrigger(t *testing.T) {
	_, fail := answerTable("testdata/one_col.csv", false, false)
	if fail == nil || !strings.Contains(message.CannotStartText(*fail), "第 1 行不合规") {
		t.Fatalf("应当指到第 1 行：%v", fail)
	}
	if !strings.Contains(message.CannotStartText(*fail), "缺触发词") {
		t.Fatalf("话里要说清缺触发词：%s", message.CannotStartText(*fail))
	}
}

// 行号照文本数：注释行与空行照样占号。
func TestAnswerTableReportsRealLineNumber(t *testing.T) {
	_, fail := answerTable("testdata/bad_line.csv", false, false)
	if fail == nil {
		t.Fatal("应当报错")
	}
	if !strings.Contains(message.CannotStartText(*fail), "第 3 行不合规") {
		t.Fatalf("第三行才是那条坏记录：%s", message.CannotStartText(*fail))
	}
}

// 正则口径下，写坏的正则当场拦下——不留到运行期静默不匹配。
func TestAnswerTableRejectsBadRegex(t *testing.T) {
	_, fail := answerTable("y,(未闭合", true, false)
	if fail == nil {
		t.Fatal("应当报错")
	}
	if !strings.Contains(message.CannotStartText(*fail), "触发词不是合法正则") {
		t.Fatalf("话里要说清是正则的问题：%s", message.CannotStartText(*fail))
	}
	// 同一个值在子串口径下是合法触发词：括号只是普通字符。
	if _, ok := answerTable("y,(未闭合", false, false); ok != nil {
		t.Fatalf("子串口径下不该拦：%v", message.CannotStartText(*ok))
	}
}

// 没给 -a 时交回空表，不是错。
func TestAnswerTableEmpty(t *testing.T) {
	answers, fail := answerTable("", false, false)
	if fail != nil || answers != nil {
		t.Fatalf("空值应当交回空表：answers=%v fail=%v", answers, fail)
	}
}

// 代填表进纸：命令与脚本两种载荷各一份，解析站读一次，执行站照着喂。
func TestAnswerTableLandsOnPaper(t *testing.T) {
	dir := writeTempConfig(t)
	manifest := writeFile(t, dir, "ok.csv", "10.0.0.1,22,root,pw\n")
	script := writeFile(t, dir, "deploy.sh", "echo hi\n")

	cfg, err := load()
	if err != nil {
		t.Fatalf("临时配置读不通：%v", err)
	}

	cmd, _, fail := buildPlan(cfg, scan([]string{"sshfleet2", "-f", manifest, "-c", "install.sh", "-a", "y,继续"}))
	if fail != nil {
		t.Fatalf("命令模式不该走到「起不来」：%v", message.CannotStartText(*fail))
	}
	if cmd.Task.Command == nil || len(cmd.Task.Command.Answers) != 1 {
		t.Fatalf("命令载荷上没带代填表：%+v", cmd.Task.Command)
	}

	sh, _, fail := buildPlan(cfg, scan([]string{"sshfleet2", "-f", manifest, "-s", script, "-a", "y,继续"}))
	if fail != nil {
		t.Fatalf("脚本模式不该走到「起不来」：%v", message.CannotStartText(*fail))
	}
	if sh.Task.Script == nil || len(sh.Task.Script.Answers) != 1 {
		t.Fatalf("脚本载荷上没带代填表：%+v", sh.Task.Script)
	}
	// 给了 -a 之后 stdin 整条让给代填：下发行的正文已经编进命令行。
	if sh.Run.Stdin != "" {
		t.Fatalf("给了 -a 时纸上那格 stdin 应当是空的，实际 %q", sh.Run.Stdin)
	}
	if !strings.Contains(sh.Run.Downlink, "base64 -d") {
		t.Fatalf("下发行该是 base64 形态：%s", sh.Run.Downlink)
	}
}

// 表外解释器：解析站拼不出下发行，只记一条事实交给闸门（本站不报错）。
func TestAnswerWithUnsupportedInterpreterRecordsProblem(t *testing.T) {
	dir := writeTempConfig(t)
	manifest := writeFile(t, dir, "ok.csv", "10.0.0.1,22,root,pw\n")
	script := filepath.Join(dir, "deploy.tcl")
	if err := os.WriteFile(script, []byte("puts hi"), 0o644); err != nil {
		t.Fatalf("造脚本失败：%v", err)
	}

	cfg, err := load()
	if err != nil {
		t.Fatalf("临时配置读不通：%v", err)
	}
	// 让 `.tcl` 回退到表外的解释器：配置里 command 仍配 bash，这里改成 echo 之外的 tclsh
	// 不在表里——直接把配置的 command 换掉。
	cfg.interpreter.Command = "tclsh"
	p, _, fail := buildPlan(cfg, scan([]string{"sshfleet2", "-f", manifest, "-s", script, "-a", "y,继续"}))
	if fail != nil {
		t.Fatalf("解析站不该自己报错：%v", message.CannotStartText(*fail))
	}
	found := false
	for _, pr := range p.Problems {
		if pr.Kind == contract.ProblemInterpreterUnsupported && pr.Raw == "tclsh" {
			found = true
		}
	}
	if !found {
		t.Fatalf("应当记一条「解释器表外」的事实：%+v", p.Problems)
	}
	if p.Run.Downlink != "" {
		t.Fatalf("拼不出下发行时那一格应当是空的，实际 %q", p.Run.Downlink)
	}
}
