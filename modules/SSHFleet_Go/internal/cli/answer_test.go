package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sshfleet/internal/ssh"
)

// -a 的取值处理：来源判定（内联 / CSV 文件）、解析、门控、互斥与长度检查。
// 这几条都在参数合规检查阶段一次做完，执行侧直接用结果。

func TestCheckAnswerInline(t *testing.T) {
	a := &Args{Command: "sh x.sh", Answer: []string{"1,请选择架构", "2,deb/rpm,包格式"}}
	if err := checkAnswer(a, nil); err != nil {
		t.Fatalf("内联值应解析通过：%v", err)
	}
	if len(a.Answers) != 2 {
		t.Fatalf("应解析出 2 条代填，实际 %d 条", len(a.Answers))
	}
	if a.Answers[0].Value != "1" || len(a.Answers[0].Triggers) != 1 || a.Answers[0].Triggers[0] != "请选择架构" {
		t.Fatalf("第 1 条解析不对：%+v", a.Answers[0])
	}
	if len(a.Answers[1].Triggers) != 2 || a.Answers[1].Triggers[0] != "deb/rpm" {
		t.Fatalf("一条挂多个触发词应全收：%+v", a.Answers[1])
	}
}

// 值里的 \n（字面量）是行分隔符：一条一行，完全照 CSV 的行格式；空行与 # 行跳过。
func TestCheckAnswerInlineMultiline(t *testing.T) {
	a := &Args{Command: "sh x.sh", Answer: []string{`1,请选择架构\n#注释\n\n2,请选择包格式`}}
	if err := checkAnswer(a, nil); err != nil {
		t.Fatalf("多行内联值应解析通过：%v", err)
	}
	if len(a.Answers) != 2 {
		t.Fatalf("应解析出 2 条代填，实际 %d 条：%+v", len(a.Answers), a.Answers)
	}
	if a.Answers[0].Value != "1" || a.Answers[1].Value != "2" {
		t.Fatalf("行序即条目序：%+v", a.Answers)
	}
	if len(a.AnswerFiles) != 0 {
		t.Fatalf("内联值不应记进 AnswerFiles：%v", a.AnswerFiles)
	}
}

// 真实换行与字面 \n 等价（有些 shell 会把真实换行放进引号里传进来）。
func TestCheckAnswerInlineRealNewline(t *testing.T) {
	a := &Args{Command: "sh x.sh", Answer: []string{"1,请选择架构\n2,请选择包格式"}}
	if err := checkAnswer(a, nil); err != nil {
		t.Fatalf("真实换行应与 \\n 等价：%v", err)
	}
	if len(a.Answers) != 2 {
		t.Fatalf("应解析出 2 条代填，实际 %d 条", len(a.Answers))
	}
}

// 所有行都被跳过（空行 / # 行）：报没有可用条目，不静默跑空表。
func TestCheckAnswerInlineAllRowsSkipped(t *testing.T) {
	a := &Args{Command: "sh x.sh", Answer: []string{`#全是注释,a,b\n\n`}}
	err := checkAnswer(a, nil)
	if err == nil || !strings.Contains(err.Error(), "没有可用的代填条目") {
		t.Fatalf("应报没有可用的代填条目，实际：%v", err)
	}
}

// 第一段留空是合法写法：代表只发一个回车。
func TestCheckAnswerEmptyValueMeansEmptyLine(t *testing.T) {
	a := &Args{Command: "sh x.sh", Answer: []string{",继续"}}
	if err := checkAnswer(a, nil); err != nil {
		t.Fatalf("留空的第一段应合法：%v", err)
	}
	if a.Answers[0].Value != "" || a.Answers[0].Triggers[0] != "继续" {
		t.Fatalf("留空应解析成空内容 + 触发词：%+v", a.Answers[0])
	}
}

func TestCheckAnswerRejectsMissingTrigger(t *testing.T) {
	for _, raw := range []string{"1", "1,", "1,,x"} {
		a := &Args{Command: "sh x.sh", Answer: []string{raw}}
		err := checkAnswer(a, nil)
		if err == nil {
			t.Fatalf("%q 应被拒绝（触发词是唯一的门控）", raw)
		}
	}
}

// 不含逗号又不指向已有文件：按 -f 的同一形状判定，报「文件不存在」。
func TestCheckAnswerValueWithoutCommaIsFile(t *testing.T) {
	a := &Args{Command: "sh x.sh", Answer: []string{"不存在的文件"}}
	err := checkAnswer(a, nil)
	if err == nil || !strings.Contains(err.Error(), "文件不存在") {
		t.Fatalf("应报文件不存在，实际：%v", err)
	}
}

func TestCheckAnswerCSVFile(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "answer.csv")
	content := "# 注释行\n\n1,请选择架构\n2,deb/rpm,包格式\n"
	if err := os.WriteFile(file, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	a := &Args{Command: "sh x.sh", Answer: []string{"3,服务端口", file}}
	if err := checkAnswer(a, nil); err != nil {
		t.Fatalf("文件与内联应能混用：%v", err)
	}
	if len(a.Answers) != 3 {
		t.Fatalf("应解析出 3 条代填，实际 %d 条", len(a.Answers))
	}
	// 条目顺序即命令行给出顺序：先内联的那条，再文件里的两条
	if a.Answers[0].Value != "3" || a.Answers[1].Value != "1" || a.Answers[2].Value != "2" {
		t.Fatalf("条目顺序不对：%+v", a.Answers)
	}
	// 文件来源记进 AnswerFiles（归档备份用），内联值不记
	if len(a.AnswerFiles) != 1 || a.AnswerFiles[0] != file {
		t.Fatalf("应记录文件来源 %s，实际：%v", file, a.AnswerFiles)
	}
}

// 记事本存的 CSV 默认带 UTF-8 BOM：留着会让首行的 # 注释判断失效、整行被当数据。
func TestCheckAnswerCSVWithBOM(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "answer.csv")
	if err := os.WriteFile(file, []byte("\ufeff# 注释\n1,请选择架构\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	a := &Args{Command: "sh x.sh", Answer: []string{file}}
	if err := checkAnswer(a, nil); err != nil {
		t.Fatalf("带 BOM 的代填文件应正常解析：%v", err)
	}
	if len(a.Answers) != 1 || a.Answers[0].Value != "1" {
		t.Fatalf("应解析出 1 条代填：%+v", a.Answers)
	}
}

func TestCheckAnswerCSVReportsLine(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "answer.csv")
	if err := os.WriteFile(file, []byte("1,架构\n3\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	a := &Args{Command: "sh x.sh", Answer: []string{file}}
	err := checkAnswer(a, nil)
	if err == nil || !strings.Contains(err.Error(), "第 2 行") {
		t.Fatalf("应报出出错行号，实际：%v", err)
	}
}

func TestCheckAnswerCSVEmpty(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "answer.csv")
	if err := os.WriteFile(file, []byte("# 只有注释\n\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	a := &Args{Command: "sh x.sh", Answer: []string{file}}
	if err := checkAnswer(a, nil); err == nil {
		t.Fatal("一条代填都没有的文件应报错")
	}
}

// 互斥：三种组合都明确报错，不静默忽略。
func TestCheckAnswerExclusive(t *testing.T) {
	cases := map[string]*Args{
		"--no-bash": {Command: "sh x.sh", NoBash: true, Answer: []string{"1,架构"}},
		"-u":        {Upload: "/x", Path: "/y/", Answer: []string{"1,架构"}},
		"-d":        {Download: "/x", Path: "/y", Answer: []string{"1,架构"}},
	}
	for name, a := range cases {
		if err := checkAnswer(a, nil); err == nil || !strings.Contains(err.Error(), "-a 不能和") {
			t.Fatalf("%s 时应明确报错，实际：%v", name, err)
		}
	}
}

// 长度检查：量的是最终下发行，仅 -a 运行执行；超限在本地拦下。
func TestCheckAnswerLength(t *testing.T) {
	ok := &Args{Command: strings.Repeat("a", 1000), Answer: []string{"1,架构"}}
	if err := checkAnswer(ok, nil); err != nil {
		t.Fatalf("短命令不该触发长度检查：%v", err)
	}

	long := &Args{Command: strings.Repeat("a", 100000), Answer: []string{"1,架构"}}
	err := checkAnswer(long, nil)
	if err == nil || !strings.Contains(err.Error(), "太长") {
		t.Fatalf("超限应报错，实际：%v", err)
	}
}

// 脚本模式下长度按脚本正文算（命令模式算的是 a.Command）。
func TestCheckAnswerLengthUsesScriptBody(t *testing.T) {
	a := &Args{Script: "D:/x/big.sh", Answer: []string{"1,架构"}}
	err := checkAnswer(a, []byte(strings.Repeat("a", 100000)))
	if err == nil || !strings.Contains(err.Error(), "太长") {
		t.Fatalf("脚本正文超限应报错，实际：%v", err)
	}
}

// 正则模式下触发词要能编译：语法错在本地报错，不留到运行期静默不匹配。
func TestCheckAnswerRejectsBadRegex(t *testing.T) {
	a := &Args{
		Command: "sh x.sh",
		Answer:  []string{`1,arch[`},
		Match:   ssh.MatchOptions{Regex: true},
	}
	err := checkAnswer(a, nil)
	if err == nil || !strings.Contains(err.Error(), "arch[") {
		t.Fatalf("非法正则应报错且带上原文，实际：%v", err)
	}
}
