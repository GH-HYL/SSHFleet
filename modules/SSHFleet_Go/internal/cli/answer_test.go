package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sshfleet/internal/config"
)

// -a 的取值处理：来源判定（文件 / 内联文本）、解析、门控、互斥与长度检查。
// 这几条都在参数合规检查阶段一次做完，执行侧直接用结果。
//
// CSV 解析口径与 -f 同构（两处都走 common.ReadCSVRows），这里测的是代填自己的
// 列语义与门控；解析口径本身由 common 侧的单测覆盖。

// answerTestCfg 带 -a 的用例要用的配置：-a 要求解释器会读程序文本，
// 空串会被 checkAnswerInterpreter 拦下——那是另一件事，不该混进取值判定里。
func answerTestCfg() *config.Config {
	cfg := helpTestCfg()
	cfg.Interpreter.Command = "bash"
	return cfg
}

// answerArgs 用命令行入口造 -a 的入参：与 main 一样，Parse 之后紧跟合规检查。
func answerArgs(t *testing.T, args ...string) (*Args, error) {
	t.Helper()
	a, err := Parse(answerTestCfg(), "test", args)
	if err != nil {
		t.Fatalf("Parse 不该在这里报错：%v", err)
	}
	return a, CheckArguments(a)
}

// answerErr 一条内联代填从命令行入口走一遍，只返回合规检查的报错。
func answerErr(t *testing.T, value string) error {
	t.Helper()
	_, err := answerArgs(t, "-f", "172.28.118.49,22,root,pw", "-c", "sh x.sh", "-a", value)
	return err
}

// answerOK 一条内联代填从命令行入口走一遍，要求通过并把解析结果交回来。
func answerOK(t *testing.T, value string) *Args {
	t.Helper()
	a, err := answerArgs(t, "-f", "172.28.118.49,22,root,pw", "-c", "sh x.sh", "-a", value)
	if err != nil {
		t.Fatalf("-a %q 应通过合规检查：%v", value, err)
	}
	return a
}

// 代填报错必须点名 -a / 代填，并说清是文件还是内联值。
// 此前几条只甩一个文件名（如「X 是二进制文件」），使用者不知道是哪个参数带进来的
// （2026-10-15 审计实测）。
func TestAnswerErrorsNameAnswerFlag(t *testing.T) {
	dir := t.TempDir()
	binPath := filepath.Join(dir, "a.bin")
	if err := os.WriteFile(binPath, []byte{0x00, 0x01, 0x02}, 0o644); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name string
		args []string
	}{
		{"代填文件是二进制", []string{"-f", "172.28.118.49,22,root,pw", "-c", "sh x.sh", "-a", binPath}},
		{"代填文件不可读", []string{"-f", "172.28.118.49,22,root,pw", "-c", "sh x.sh", "-a", filepath.Join(dir, "nope.csv")}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a, err := Parse(helpTestCfg(), "test", c.args)
			if err != nil {
				t.Fatalf("Parse 不该在这里报错：%v", err)
			}
			err = CheckArguments(a)
			if err == nil {
				t.Fatal("应报错，实际通过")
			}
			if !strings.Contains(err.Error(), "-a") {
				t.Fatalf("报错应点名 -a，实际：%v", err)
			}
		})
	}
}

// 内联值的一条不合规时，报错要说清是 -a 的值、第几行。
func TestAnswerInlineLineErrorNamesFlagAndLine(t *testing.T) {
	// 只有一列，缺触发词（-c 与 -a 的组合能通过模式互斥检查，走到 -a 的解析）
	a, err := Parse(helpTestCfg(), "test", []string{
		"-f", "172.28.118.49,22,root,pw", "-c", "sh x.sh", "-a", "1",
	})
	if err != nil {
		t.Fatalf("Parse 不该在这里报错：%v", err)
	}
	err = CheckArguments(a)
	if err == nil {
		t.Fatal("只有一列的代填应报错")
	}
	if !strings.Contains(err.Error(), "-a") {
		t.Fatalf("报错应点名 -a，实际：%v", err)
	}
}

// 一列与空触发词这两种错误，内层要说清这一行差在哪、该怎么改。
// 外层已由 answerLineError 带上了 -a、文件与行号，这里断言的是内层那句话。
func TestAnswerRowErrorsSayWhatIsMissing(t *testing.T) {
	cases := []struct {
		name string
		body string
		want string
	}{
		{"只有一列", "1\n", "这一行缺触发词，至少要两列"},
		// `1,架构,,` 的第 3、4 列都是空触发词；报第一个，即第 3 列
		{"触发词为空", "1,架构,,\n", "第 3 列的触发词是空的"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			file := filepath.Join(t.TempDir(), "answer.csv")
			if err := os.WriteFile(file, []byte(c.body), 0o644); err != nil {
				t.Fatal(err)
			}
			err := answerErr(t, file)
			if err == nil {
				t.Fatal("应报错")
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Fatalf("报错应含 %q，实际：%v", c.want, err)
			}
		})
	}
}

// 内联值只写一条：`;;` 是普通字符，不会被切成多条（ADR-0023）。
func TestCheckAnswerInlineIsOneRecord(t *testing.T) {
	a, err := Parse(answerTestCfg(), "test", []string{
		"-f", "172.28.118.49,22,root,pw", "-c", "sh x.sh", "-a", `1,请选择架构;;deb`,
	})
	if err != nil {
		t.Fatalf("Parse 不该在这里报错：%v", err)
	}
	if err := CheckArguments(a); err != nil {
		t.Fatalf("内联一条代填应解析通过：%v", err)
	}
	if len(a.Answers) != 1 {
		t.Fatalf("应解析出 1 条代填，实际 %d 条：%+v", len(a.Answers), a.Answers)
	}
	if a.Answers[0].Value != "1" || len(a.Answers[0].Triggers) != 1 || a.Answers[0].Triggers[0] != "请选择架构;;deb" {
		t.Fatalf("`;;` 应留在触发词里、只有一条：%+v", a.Answers[0])
	}
	if a.AnswerFile != "" {
		t.Fatalf("内联值不该记成文件来源：%s", a.AnswerFile)
	}
}

// 内联值里出现换行：直接拒绝，原因只说一件，去路放在提示行（ADR-0023）。
func TestCheckAnswerInlineRejectsRealNewline(t *testing.T) {
	cases := map[string]string{
		"两条":   "1,请选择架构\n2,请选择包格式",
		"CRLF": "1,请选择架构\r\n2,请选择包格式",
	}
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			a, err := Parse(answerTestCfg(), "test", []string{
				"-f", "172.28.118.49,22,root,pw", "-c", "sh x.sh", "-a", raw,
			})
			if err != nil {
				t.Fatalf("Parse 不该在这里报错：%v", err)
			}
			err = CheckArguments(a)
			if err == nil {
				t.Fatal("内联值含换行应被拒绝")
			}
			want := "-a 的内联值里不能有换行：内联只写一条\n" +
				"提示：把代填写进 CSV 文件，再用 -a 指向它；内容里要回车也只能写文件"
			if err.Error() != want {
				t.Fatalf("文案应为 %q，实际：%q", want, err.Error())
			}
		})
	}
}

// 字面 `\n` 与 `;;` 都只是普通字符：整条记录原样进字段，不切、不转义。
func TestCheckAnswerKeepsSemicolonPairAndLiteralBackslashN(t *testing.T) {
	a, err := Parse(answerTestCfg(), "test", []string{
		"-f", "172.28.118.49,22,root,pw", "-c", "sh x.sh", "-a", `D:\new\tool.exe;;1,请输入路径`,
	})
	if err != nil {
		t.Fatalf("Parse 不该在这里报错：%v", err)
	}
	if err := CheckArguments(a); err != nil {
		t.Fatalf("应解析通过：%v", err)
	}
	if len(a.Answers) != 1 {
		t.Fatalf("应只有 1 条代填，实际 %d 条：%+v", len(a.Answers), a.Answers)
	}
	if got := a.Answers[0].Value; got != `D:\new\tool.exe;;1` {
		t.Fatalf("值应原样保留（含反斜杠与 `;;`），实际 %q", got)
	}
	if got := a.Answers[0].Triggers[0]; got != "请输入路径" {
		t.Fatalf("触发词不对：%q", got)
	}
}

// 内联值只解析出一列（引号里才有逗号）：仍按「第 1 行不合规」报缺触发词，
// 不落到「文件不存在」（ADR-0023）。
func TestCheckAnswerInlineOneColumn(t *testing.T) {
	err := answerErr(t, `"a,b"`)
	if err == nil {
		t.Fatal("缺触发词应报错")
	}
	if !strings.Contains(err.Error(), "第 1 行不合规") {
		t.Fatalf("应按行报列数问题，实际：%v", err)
	}
	if strings.Contains(err.Error(), "文件不存在") {
		t.Fatalf("不该落到「文件不存在」：%v", err)
	}
}

// 代填内容里的反斜杠是普通字符：Windows 路径常含 `\n`，不会被当成换行（ADR-0023）。
func TestCheckAnswerInlineKeepsBackslashN(t *testing.T) {
	a := answerOK(t, `D:\new\tool.exe,请输入安装路径`)
	if len(a.Answers) != 1 {
		t.Fatalf("应只有 1 条代填，实际 %d 条：%+v", len(a.Answers), a.Answers)
	}
	if a.Answers[0].Value != `D:\new\tool.exe` {
		t.Fatalf("代填内容应原样保留，实际：%+v", a.Answers)
	}
}

// 引号是 CSV 的语法：引号里的逗号归同一列，不当分隔符——内联与文件同一口径。
func TestCheckAnswerQuotedComma(t *testing.T) {
	a := answerOK(t, `"a,b",请选择架构`)
	if a.Answers[0].Value != "a,b" || a.Answers[0].Triggers[0] != "请选择架构" {
		t.Fatalf("引号里的逗号应归同一列：%+v", a.Answers[0])
	}
}

// 一份取值里一条代填都没有：报出来，不静默跑空表。
func TestCheckAnswerNoEntries(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "answer.csv")
	if err := os.WriteFile(file, []byte("#只有注释,也在注释里\n\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for name, raw := range map[string]string{
		"内联": `#只有注释,也在注释里;;;;`,
		"文件": file,
	} {
		err := answerErr(t, raw)
		if err == nil || !strings.Contains(err.Error(), "没有一条代填") {
			t.Fatalf("%s：应报没有一条代填，实际：%v", name, err)
		}
	}
}

// 第一段留空是合法写法：代表只发一个回车。
func TestCheckAnswerEmptyValueMeansEmptyLine(t *testing.T) {
	a := answerOK(t, ",继续")
	if a.Answers[0].Value != "" || a.Answers[0].Triggers[0] != "继续" {
		t.Fatalf("留空应解析成空内容 + 触发词：%+v", a.Answers[0])
	}
}

func TestCheckAnswerRejectsMissingTrigger(t *testing.T) {
	for _, raw := range []string{"1", "1,", "1,,x"} {
		err := answerErr(t, raw)
		if err == nil {
			t.Fatalf("%q 应被拒绝（触发词是唯一的门控）", raw)
		}
	}
}

// 不含逗号又不指向已有文件：照 -f 的同一形状判定，报「读不到」+ 系统原文 + 去路。
func TestCheckAnswerValueWithoutCommaIsFile(t *testing.T) {
	err := answerErr(t, "不存在的文件")
	if err == nil || !strings.Contains(err.Error(), "参数指定的文件读不到") {
		t.Fatalf("应报文件读不到，实际：%v", err)
	}
	if !strings.Contains(err.Error(), "\n原因：") || !strings.Contains(err.Error(), "\n提示：") {
		t.Fatalf("三行套应齐（原因 / 提示），实际：%v", err)
	}
}

// 文件来源：注释行与空行跳过，行号按原文数（不是跳过后的序号）。
func TestCheckAnswerCSVFile(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "answer.csv")
	content := "# 注释行\n\n1,请选择架构\n2,deb/rpm,包格式\n"
	if err := os.WriteFile(file, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	a := answerOK(t, file)
	if len(a.Answers) != 2 {
		t.Fatalf("应解析出 2 条代填，实际 %d 条", len(a.Answers))
	}
	if a.Answers[0].Value != "1" || a.Answers[1].Value != "2" {
		t.Fatalf("行序即条目序：%+v", a.Answers)
	}
	if a.AnswerFile != file {
		t.Fatalf("文件来源应记进 AnswerFile（归档备份用），实际：%s", a.AnswerFile)
	}
}

// 代填文件里的换行是内容的一部分：一条代填要打两行再回车，只能写成文件
// （内联值不许有换行，见 ADR-0023）。这里钉住文件这条路没被内联侧改动波及。
func TestCheckAnswerCSVFileKeepsRealNewline(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "answer.csv")
	if err := os.WriteFile(file, []byte("\"第一行\n第二行\",请选择架构\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	a := answerOK(t, file)
	if len(a.Answers) != 1 {
		t.Fatalf("引号里的换行不另算一条，实际 %d 条", len(a.Answers))
	}
	if got := a.Answers[0].Value; got != "第一行\n第二行" {
		t.Fatalf("内容里的换行应原样保留，实际 %q", got)
	}
}

// 记事本存的 CSV 默认带 UTF-8 BOM：留着会让首行的 # 注释判断失效、整行被当数据。
func TestCheckAnswerCSVWithBOM(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "answer.csv")
	if err := os.WriteFile(file, []byte("\ufeff# 注释\n1,请选择架构\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	a := answerOK(t, file)
	if len(a.Answers) != 1 || a.Answers[0].Value != "1" {
		t.Fatalf("应解析出 1 条代填：%+v", a.Answers)
	}
}

func TestCheckAnswerCSVReportsLine(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "answer.csv")
	// 第 2 行只有一列（缺触发词）：报出来的行号要是原文里的行号
	if err := os.WriteFile(file, []byte("1,架构\n3\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := answerErr(t, file)
	if err == nil || !strings.Contains(err.Error(), "第 2 行") {
		t.Fatalf("应报出出错行号，实际：%v", err)
	}
}

// 引号不配对：报语法错，带上行号（文案与清单侧同口径）。
func TestCheckAnswerCSVBadQuotes(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "answer.csv")
	if err := os.WriteFile(file, []byte("1,架构\n2,\"包格式\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := answerErr(t, file)
	if err == nil || !strings.Contains(err.Error(), "第 2 行") {
		t.Fatalf("引号不配对应报第 2 行，实际：%v", err)
	}
}

// 代填文件与清单文件同一口径：二进制文件直接拦下，不让它变成一堆乱码触发词。
func TestCheckAnswerCSVBinary(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "answer.bin")
	if err := os.WriteFile(file, []byte("1,架构\x00\x01\x02"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := answerErr(t, file)
	if err == nil || !strings.Contains(err.Error(), "二进制") {
		t.Fatalf("二进制文件应被拦下，实际：%v", err)
	}
}

func TestCheckAnswerCSVEmpty(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "answer.csv")
	if err := os.WriteFile(file, []byte("# 只有注释\n\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := answerErr(t, file); err == nil {
		t.Fatal("一条代填都没有的文件应报错")
	}
}

// 互斥：三种组合都明确报错，不静默忽略。
//
// -u / -d 走命令行时，前置校验（上传文件得存在、下载目录得是已有目录）会先拦住——
// 它们与 -a 的互斥因此改由配置层直接调这一条专用校验来钉，不靠制造一个真目录。
func TestCheckAnswerExclusive(t *testing.T) {
	_, err := answerArgs(t, "-c", "sh x.sh", "--no-shell", "-a", "1,架构")
	if err == nil || !strings.Contains(err.Error(), "-a 不能和 --no-shell") {
		t.Fatalf("--no-shell 时应明确报错，实际：%v", err)
	}

	for name, a := range map[string]*Args{
		"-u": {Upload: "/x", Path: "/y/", Answer: "1,架构"},
		"-d": {Download: "/x", Path: "/y", Answer: "1,架构"},
	} {
		if err := checkAnswerExclusive(a); err == nil || !strings.Contains(err.Error(), "-a 不能和") {
			t.Fatalf("%s 时应明确报错，实际：%v", name, err)
		}
	}
}

// 长度检查：量的是最终下发行，仅 -a 运行执行；超限在本地拦下。
//
// 下发行里编进去的是命令 / 脚本正文（代填内容与触发词不进这一行），所以撑爆它的
// 得是正文。期望值取自字面量：正文 130000 字节 > 上限 122880 字节。
func TestCheckAnswerLength(t *testing.T) {
	if err := answerErr(t, "1,架构"); err != nil {
		t.Fatalf("短命令不该触发长度检查：%v", err)
	}

	_, err := answerArgs(t, "-f", "172.28.118.49,22,root,pw", "-c", strings.Repeat("a", 130000), "-a", "1,架构")
	if err == nil || !strings.Contains(err.Error(), "太长") {
		t.Fatalf("超限应报错，实际：%v", err)
	}
}

// 脚本模式下长度按脚本正文算（命令模式算的是 a.Command）。
func TestCheckAnswerLengthUsesScriptBody(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "big.sh")
	if err := os.WriteFile(script, []byte(strings.Repeat("a", 100000)), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := answerArgs(t, "-f", "172.28.118.49,22,root,pw", "-s", script, "-a", "1,架构")
	if err == nil || !strings.Contains(err.Error(), "太长") {
		t.Fatalf("脚本正文超限应报错，实际：%v", err)
	}
}

// 正则模式下触发词要能编译：语法错在本地报错，不留到运行期静默不匹配。
func TestCheckAnswerRejectsBadRegex(t *testing.T) {
	cfg := answerTestCfg()
	cfg.Interactive.Regex = true
	a, err := Parse(cfg, "test", []string{
		"-f", "172.28.118.49,22,root,pw", "-c", "sh x.sh", "-a", `1,arch[`,
	})
	if err != nil {
		t.Fatalf("Parse 不该在这里报错：%v", err)
	}
	err = CheckArguments(a)
	if err == nil || !strings.Contains(err.Error(), "arch[") {
		t.Fatalf("非法正则应报错且带上原文，实际：%v", err)
	}
}
