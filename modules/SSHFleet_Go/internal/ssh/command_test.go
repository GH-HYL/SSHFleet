package ssh

import (
	"fmt"
	"strings"
	"testing"
)

// BuildCommand 的命令构建回归。
//
// 这个函数此前收整份命令行参数，想测它得先凑齐一整套参数与配置；
// 改成收基本类型后，输入就是几行字面量，可以逐条钉住下发形态。

// 命令模式：命令文本经 stdin 直喂，命令行只剩 bash -lc '<前缀; [sudo ]bash>'。
func TestBuildCommandCommandMode(t *testing.T) {
	cmd, stdin := BuildCommand("ls -l", "", "", false, false)

	if stdin != "ls -l" {
		t.Fatalf("命令应经 stdin 直喂，实际 %q", stdin)
	}
	if !strings.HasPrefix(cmd, "bash -lc ") {
		t.Fatalf("应套一层登录 shell，实际 %q", cmd)
	}
	if !strings.Contains(cmd, envPrefix) {
		t.Fatalf("应带上语言环境前缀，实际 %q", cmd)
	}
	if strings.Contains(cmd, "sudo") {
		t.Fatalf("direct 身份不该出现 sudo，实际 %q", cmd)
	}
	if !strings.HasSuffix(cmd, "bash'") {
		t.Fatalf("内层应是 bash，实际 %q", cmd)
	}
}

// sudo 身份：内层命令前加 sudo。
func TestBuildCommandSudo(t *testing.T) {
	cmd, _ := BuildCommand("whoami", "", "", false, true)
	if !strings.Contains(cmd, "sudo bash") {
		t.Fatalf("sudo 身份应加 sudo，实际 %q", cmd)
	}
}

// --nobash：命令模式原样下发，不套 shell、不喂 stdin。
func TestBuildCommandNoBash(t *testing.T) {
	cmd, stdin := BuildCommand("raw-cmd --flag", "", "", true, false)
	if cmd != "raw-cmd --flag" || stdin != "" {
		t.Fatalf("--nobash 应原样下发且不喂 stdin，实际 cmd=%q stdin=%q", cmd, stdin)
	}
}

// --nobash 只管命令模式：脚本模式下仍应走登录 shell。
func TestBuildCommandNoBashIgnoredForScript(t *testing.T) {
	cmd, stdin := BuildCommand("", "echo script", "bash", true, false)
	if cmd == "echo script" || !strings.HasPrefix(cmd, "bash -lc ") {
		t.Fatalf("脚本模式不该被 --nobash 影响，实际 %q", cmd)
	}
	if stdin != "echo script" {
		t.Fatalf("脚本内容应经 stdin 直喂，实际 %q", stdin)
	}
}

// 脚本模式：用给定解释器，脚本内容经 stdin 直喂。
func TestBuildCommandScriptMode(t *testing.T) {
	cases := []struct{ interpreter, wantInner string }{
		{"bash", "bash"},
		{"python3", "python3"},
	}
	for _, c := range cases {
		cmd, stdin := BuildCommand("", "print('hi')", c.interpreter, false, false)
		if stdin != "print('hi')" {
			t.Fatalf("脚本内容应经 stdin 直喂，实际 %q", stdin)
		}
		if !strings.Contains(cmd, c.wantInner) {
			t.Fatalf("应使用解释器 %q，实际 %q", c.interpreter, cmd)
		}
	}
}

// shellQuote：内部单引号按 '\” 转义，保证整串作为 bash -lc 的单个参数传递。
func TestShellQuote(t *testing.T) {
	cases := []struct{ in, want string }{
		{"plain", "'plain'"},
		{"it's", `'it'\''s'`},
		{"a'b'c", `'a'\''b'\''c'`},
		{"", "''"},
	}
	for _, c := range cases {
		if got := shellQuote(c.in); got != c.want {
			t.Fatalf("shellQuote(%q) 应为 %q，实际 %q", c.in, c.want, got)
		}
	}
}

// 命令模式的命令走 stdin，**不进命令行**——命令行里只有固定形态的
// `bash -lc '前缀; bash'`，故命令里的引号无需转义、也不会逃出参数边界。
// （早期实现走 base64 命令行通道，才有转义套娃；改用 stdin 后这条约束消失。）
func TestBuildCommandCommandTextNotOnCommandLine(t *testing.T) {
	cmd, stdin := BuildCommand(`echo 'hi'`, "", "", false, false)

	if strings.Contains(cmd, "hi") {
		t.Fatalf("命令原文不该出现在命令行里（已改走 stdin）：%q", cmd)
	}
	if stdin != `echo 'hi'` {
		t.Fatalf("命令原文应原样进 stdin，实际 %q", stdin)
	}
	// 命令行形态固定：bash -lc '<单引号包裹的前缀; bash>'
	if !strings.HasPrefix(cmd, "bash -lc '") || !strings.HasSuffix(cmd, "'") {
		t.Fatalf("命令行形态不符：%q", cmd)
	}
	if strings.Count(cmd, "'") != 2 {
		t.Fatalf("命令行的单引号应恰好一对（包裹整串），实际 %q", cmd)
	}
}

// QuoteForShell：只给「会被 shell 二次拆词 / 展开」的参数补引号。
// 判据是拆词风险，不是「原样里有没有空格」——纯单字命令、逗号内联清单都不该被引号包住。
func TestQuoteForShell(t *testing.T) {
	cases := []struct{ in, want string }{
		// 不需要引号：无 shell 特殊字符
		{"-c", "-c"},
		{"pwd", "pwd"},
		{"/x/a.csv", "/x/a.csv"},
		{"172.28.118.49,22,root", "172.28.118.49,22,root"}, // 内联清单：逗号在安全集内
		{"C:/Users/Administrator/keys/id", "C:/Users/Administrator/keys/id"},
		{"-k", "-k"},
		// 需要引号：含空格（拆词）
		{"who -b", "'who -b'"},
		{"df -h; free -m", "'df -h; free -m'"},
		{"D:/My Dir/a.csv", "'D:/My Dir/a.csv'"},
		{"trailing ", "'trailing '"},
		// 需要引号：shell 会展开的特殊字符
		{"*.csv", "'*.csv'"},
		{"~/x", "'~/x'"},
		{"$HOME", "'$HOME'"},
		{"a|b", "'a|b'"},
		{"(x)", "'(x)'"},
		// 内部单引号按 '\” 转义
		{"it's here", `'it'\''s here'`},
		// 空参数显式给一对引号（不留一个看不见的空位）
		{"", "''"},
	}
	for _, c := range cases {
		if got := QuoteForShell(c.in); got != c.want {
			t.Fatalf("QuoteForShell(%q) 应为 %q，实际 %q", c.in, c.want, got)
		}
	}
}

// 打印的命令行必须能照抄重跑：`-c 'who -b'` 的 argv 拿着时补回引号，
// 平铺成 `-c who -b` 就是另一条命令（`-b` 会变成多余参数）。
func TestQuoteForShellRestoresBrokenArgv(t *testing.T) {
	argv := []string{"SSHFleet.exe", "-f", "nodes.csv", "-c", "who -b"}
	got := make([]string, 0, len(argv))
	for _, s := range argv {
		got = append(got, QuoteForShell(s))
	}
	if want := "SSHFleet.exe -f nodes.csv -c 'who -b'"; strings.Join(got, " ") != want {
		t.Fatalf("还原后应为 %q，实际 %q", want, strings.Join(got, " "))
	}
}

// DescribeCommand 命令模式：交代处理方式 + 执行身份 + 下发行 + 经 stdin 的内容。
func TestDescribeCommandCommandMode(t *testing.T) {
	text := DescribeCommand(DescribeInput{
		Command:  "who -b",
		Identity: "登录用户（direct）",
	})

	for _, want := range []string{
		"经 stdin 直喂",
		"登录用户（direct）",
		"bash -lc 'export LC_ALL=en_US.UTF-8 LANG=en_US.UTF-8; bash'",
		"who -b",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("应有 %q，实际：\n%s", want, text)
		}
	}
	// 下发行的内层前缀必须与真下发的命令同源
	login, _ := BuildCommand("who -b", "", "", false, false)
	if !strings.Contains(text, login) {
		t.Fatalf("交代的下发行应与 BuildCommand 同源\n期望含：%s\n实际：\n%s", login, text)
	}
}

// DescribeCommand sudo 身份：下发行里带上 sudo。
func TestDescribeCommandSudoIdentity(t *testing.T) {
	text := DescribeCommand(DescribeInput{
		Command: "id -u",
		AsRoot:  true,
	})
	login, _ := BuildCommand("id -u", "", "", false, true)
	if !strings.Contains(text, login) {
		t.Fatalf("sudo 时下发行应与 BuildCommand 同源\n期望含：%s\n实际：\n%s", login, text)
	}
	if !strings.Contains(text, "sudo bash") {
		t.Fatalf("sudo 身份下发行应含 sudo bash，实际：\n%s", text)
	}
}

// DescribeCommand --nobash：交代「原样下发、不过 stdin」，且不出现 bash -lc。
func TestDescribeCommandNoBash(t *testing.T) {
	text := DescribeCommand(DescribeInput{Command: "raw-cmd --flag", NoBash: true})
	if !strings.Contains(text, "--nobash 原样下发") {
		t.Fatalf("应交代 --nobash 处理方式，实际：\n%s", text)
	}
	if strings.Contains(text, "bash -lc") {
		t.Fatalf("--nobash 下不该出现登录 shell 下发行，实际：\n%s", text)
	}
	if !strings.Contains(text, "raw-cmd --flag") {
		t.Fatalf("应交代原始命令，实际：\n%s", text)
	}
}

// DescribeCommand 脚本模式：交代解释器与身份，正文只打前若干行。
func TestDescribeCommandScriptMode(t *testing.T) {
	body := "echo 1\necho 2"
	text := DescribeCommand(DescribeInput{
		ScriptPath:  "/x/t.sh",
		ScriptBody:  body,
		Interpreter: "bash",
		Identity:    "root（sudo 提权）",
		AsRoot:      true,
	})
	for _, want := range []string{
		"脚本内容经 stdin 直喂",
		"bash",
		"root（sudo 提权）",
		"bash -lc 'export LC_ALL=en_US.UTF-8 LANG=en_US.UTF-8; sudo bash'",
		"| echo 1",
		"| echo 2",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("应有 %q，实际：\n%s", want, text)
		}
	}
}

// 脚本正文超长时只打前 maxPreviewLines 行，并报清省略了多少行。
func TestDescribeCommandScriptPreviewCapped(t *testing.T) {
	var sb strings.Builder
	for i := 1; i <= maxPreviewLines+7; i++ {
		fmt.Fprintf(&sb, "line%d\n", i)
	}
	text := DescribeCommand(DescribeInput{
		ScriptPath:  "/x/t.sh",
		ScriptBody:  strings.TrimSpace(sb.String()),
		Interpreter: "bash",
	})

	if !strings.Contains(text, "| line1") || !strings.Contains(text, fmt.Sprintf("| line%d", maxPreviewLines)) {
		t.Fatalf("前 %d 行都应打印，实际：\n%s", maxPreviewLines, text)
	}
	if strings.Contains(text, fmt.Sprintf("| line%d", maxPreviewLines+1)) {
		t.Fatalf("超过上限的行不该打印，实际：\n%s", text)
	}
	if !strings.Contains(text, "……其余 7 行省略") {
		t.Fatalf("应报清省略行数，实际：\n%s", text)
	}
}

// 上传 / 下载模式没有命令可交代。
func TestDescribeCommandNoCommand(t *testing.T) {
	if text := DescribeCommand(DescribeInput{}); text != "" {
		t.Fatalf("无命令时应给空串，实际 %q", text)
	}
}
