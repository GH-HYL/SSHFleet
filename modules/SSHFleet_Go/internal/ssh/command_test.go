package ssh

import (
	"strings"
	"testing"
)

// BuildCommand 的命令构建回归。
//
// 这个函数此前收整份命令行参数，想测它得先凑齐一整套参数与配置；
// 改成收基本类型后，输入就是几行字面量，可以逐条钉住下发形态。

// 命令模式：命令文本经 stdin 直喂，命令行只剩 sh -c '<前缀; [sudo ]解释器>'。
func TestBuildCommandCommandMode(t *testing.T) {
	const env = "export LC_ALL=C;"
	cmd, stdin := BuildCommand("ls -l", "", "", "bash", env, false, false, false)

	if stdin != "ls -l" {
		t.Fatalf("命令应经 stdin 直喂，实际 %q", stdin)
	}
	if !strings.HasPrefix(cmd, "sh -c ") {
		t.Fatalf("应套一层外壳，实际 %q", cmd)
	}
	if !strings.Contains(cmd, env) {
		t.Fatalf("应带上环境前缀，实际 %q", cmd)
	}
	if strings.Contains(cmd, "sudo") {
		t.Fatalf("direct 身份不该出现 sudo，实际 %q", cmd)
	}
	if !strings.HasSuffix(cmd, "bash'") {
		t.Fatalf("内层应是 bash，实际 %q", cmd)
	}
}

// wantEncodingFragment 编码判断片段，逐字手写——期望值不从实现取。
//
// 形态：UTF-8 就不动手；否则在 C.UTF-8 / en_US.UTF-8 两个中性名里挑一个确实存在的
// （大小写、连字符不计），只 export LC_CTYPE，挑不到就一个都不设。
const wantEncodingFragment = `if [ "$(locale charmap 2>/dev/null)" != UTF-8 ]; then n=$(locale -a 2>/dev/null | grep -Eixm1 "c\.utf-?8|en_us\.utf-?8"); [ -n "$n" ] && export LC_CTYPE="$n"; fi;`

// 四种下发形态各一条整串断言：片段必须原样出现在 sh -c 里、紧挨解释器、不加任何转义
// （片段里没有单引号，所以外层不会再套一层 '\''）。
func TestBuildCommandEncodingFragmentInAllForms(t *testing.T) {
	const env = `export PATH="/bin";`
	cases := []struct {
		what string
		got  string
		want string
	}{
		{"命令模式（无环境段）",
			mustCommand(t, "ls -l", "", "", "bash", "", false, false),
			"sh -c '" + wantEncodingFragment + " bash'"},
		{"命令模式（有环境段）",
			mustCommand(t, "ls -l", "", "", "bash", env, false, false),
			"sh -c '" + env + " " + wantEncodingFragment + " bash'"},
		{"sudo 模式",
			mustCommand(t, "id -u", "", "", "bash", env, false, true),
			"sh -c '" + env + " " + wantEncodingFragment + " sudo bash'"},
		{"脚本模式（补回脚本名）",
			mustCommand(t, "", "echo hi", "t.sh", "bash", env, false, false),
			"sh -c '" + env + " " + wantEncodingFragment + " bash -c " +
				"'\\''. /dev/stdin'\\'' '\\''t.sh'\\'''"},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Fatalf("%s\n want %q\n got  %q", c.what, c.want, c.got)
		}
	}
}

// wantEnglishFragment 开关打开时的片段：编码判断之后多一句把提示语钉成英文。
const wantEnglishFragment = wantEncodingFragment + " export LC_MESSAGES=C;"

// 语言开关：开、关两种产出一边一条逐字断言，差别只在那一句——编码部分不随它变。
func TestLocaleFragmentSwitch(t *testing.T) {
	if got := LocaleFragment(false); got != wantEncodingFragment {
		t.Fatalf("关：\n want %q\n got  %q", wantEncodingFragment, got)
	}
	if got := LocaleFragment(true); got != wantEnglishFragment {
		t.Fatalf("开：\n want %q\n got  %q", wantEnglishFragment, got)
	}
}

// 开关打开时下发行整串：那一句夹在片段之后、解释器之前。
func TestBuildCommandEnglishSwitch(t *testing.T) {
	const env = `export PATH="/bin";`
	want := "sh -c '" + env + " " + wantEnglishFragment + " bash'"
	if got, _ := BuildCommand("ls", "", "", "bash", env, false, false, true); got != want {
		t.Fatalf("\n want %q\n got  %q", want, got)
	}
	// 关着时不该出现那一句
	if got, _ := BuildCommand("ls", "", "", "bash", env, false, false, false); strings.Contains(got, "LC_MESSAGES") {
		t.Fatalf("关着时不该有语言开关那一句：%q", got)
	}
	// 片段里只要有一个单引号，外层 shellQuote 就会把它转义成 '\''；整串里不该出现这种痕迹
	if got, _ := BuildCommand("ls", "", "", "bash", env, false, false, true); strings.Contains(got, `'\''`) {
		t.Fatalf("片段里混进了单引号，外层引号套娃了：%q", got)
	}
}

func mustCommand(t *testing.T, command, body, name, interpreter, env string, noShell, asRoot bool) string {
	t.Helper()
	cmd, _ := BuildCommand(command, body, name, interpreter, env, noShell, asRoot, false)
	return cmd
}

// 环境前缀空也不等于「什么都不加」：编码判断照样在，内层直接接解释器，且不多出空格。
func TestBuildCommandEmptyEnvPrefix(t *testing.T) {
	want := "sh -c '" + wantEncodingFragment + " bash'"
	if cmd, _ := BuildCommand("ls", "", "", "bash", "", false, false, false); cmd != want {
		t.Fatalf("空前缀应只剩片段 + 解释器，实际 %q", cmd)
	}
	wantSudo := "sh -c '" + wantEncodingFragment + " sudo bash'"
	if cmd, _ := BuildCommand("ls", "", "", "bash", "", false, true, false); cmd != wantSudo {
		t.Fatalf("空前缀 + sudo 应为片段 + sudo + 解释器，实际 %q", cmd)
	}
	// --no-shell 不套外壳，片段不出现
	if cmd, _ := BuildCommand("ls", "", "", "bash", "", true, false, false); cmd != "ls" {
		t.Fatalf("--no-shell 应原样下发，实际 %q", cmd)
	}
}

// ExportPrefix：空表返回空串；键名排序；值一律双引号包裹，只转义值里的双引号，
// 其余字符（`$`、反斜杠）原样交目标节点按 shell 双引号语义处理。
func TestExportPrefix(t *testing.T) {
	if got := ExportPrefix(nil); got != "" {
		t.Fatalf("空表应返回空串，实际 %q", got)
	}
	cases := []struct {
		what string
		in   map[string]string
		want string
	}{
		{"键名排序 + 值一律双引号",
			map[string]string{"PATH": "/a:/b", "LC_ALL": "en_US.UTF-8", "MY": "a b"},
			`export LC_ALL="en_US.UTF-8" MY="a b" PATH="/a:/b";`},
		{"$ 不转义，留给目标节点展开",
			map[string]string{"MYPATH": "/opt/bin:$PATH"},
			`export MYPATH="/opt/bin:$PATH";`},
		{"值里的双引号转义",
			map[string]string{"Q": `say "hi"`},
			`export Q="say \"hi\"";`},
		{"反斜杠原样（\\$ 落地是字面 $）",
			map[string]string{"L": `a\$b`, "W": `C:\dir`},
			`export L="a\$b" W="C:\dir";`},
	}
	for _, c := range cases {
		if got := ExportPrefix(c.in); got != c.want {
			t.Fatalf("%s\n want %q\n got  %q", c.what, c.want, got)
		}
	}
}

// sudo 身份：内层命令前加 sudo。
func TestBuildCommandSudo(t *testing.T) {
	cmd, _ := BuildCommand("whoami", "", "", "bash", "", false, true, false)
	if !strings.Contains(cmd, "sudo bash") {
		t.Fatalf("sudo 身份应加 sudo，实际 %q", cmd)
	}
}

// --no-shell：命令模式原样下发，不套 shell、不喂 stdin。
func TestBuildCommandNoShell(t *testing.T) {
	cmd, stdin := BuildCommand("raw-cmd --flag", "", "", "", "", true, false, false)
	if cmd != "raw-cmd --flag" || stdin != "" {
		t.Fatalf("--no-shell 应原样下发且不喂 stdin，实际 cmd=%q stdin=%q", cmd, stdin)
	}
}

// --no-shell 对脚本模式同样成立（2026-10-09）：剥掉外壳与环境，只剩解释器自己那条调用
// （`<解释器> -c ". /dev/stdin" "<脚本名>"`），正文仍走 stdin、$0 仍补回。
func TestBuildCommandNoShellWithScript(t *testing.T) {
	cmd, stdin := BuildCommand("", "echo script", "hello.sh", "bash", "", true, false, false)
	if strings.HasPrefix(cmd, "sh -c ") {
		t.Fatalf("--no-shell 脚本模式不该再套外壳，实际 %q", cmd)
	}
	if strings.Contains(cmd, "export ") {
		t.Fatalf("--no-shell 不该导入环境，实际 %q", cmd)
	}
	if !strings.Contains(cmd, ". /dev/stdin") || !strings.Contains(cmd, "hello.sh") {
		t.Fatalf("应保留 source stdin 引导与脚本名，实际 %q", cmd)
	}
	if stdin != "echo script" {
		t.Fatalf("正文仍应经 stdin 直喂，实际 %q", stdin)
	}

	// 非 shell 解释器：只剩裸解释器名，正文走 stdin
	cmd, stdin = BuildCommand("", "print(1)", "t.py", "python3", "", true, false, false)
	if cmd != "python3" || stdin != "print(1)" {
		t.Fatalf("非 shell 解释器应只剩裸名字，实际 cmd=%q stdin=%q", cmd, stdin)
	}
}

// 脚本模式：用给定解释器，脚本内容经 stdin 直喂。
func TestBuildCommandScriptMode(t *testing.T) {
	cases := []struct{ interpreter, wantInner string }{
		{"bash", "bash"},
		{"python3", "python3"},
	}
	for _, c := range cases {
		cmd, stdin := BuildCommand("", "print('hi')", "", c.interpreter, "", false, false, false)
		if stdin != "print('hi')" {
			t.Fatalf("脚本内容应经 stdin 直喂，实际 %q", stdin)
		}
		if !strings.Contains(cmd, c.wantInner) {
			t.Fatalf("应使用解释器 %q，实际 %q", c.interpreter, cmd)
		}
	}
}

// 脚本模式补回脚本名（$0）：正文仍走 stdin，但名字要交回去——脚本常从自己的文件名里
// 取信息（文件名带目标 IP 的安装包），经工具跑时 $0 成了 "bash"，那些值就取空。
func TestBuildCommandScriptKeepsScriptName(t *testing.T) {
	cmd, stdin := BuildCommand("", "echo hi", "(1.2.3.4).sh", "bash", "", false, false, false)

	if stdin != "echo hi" {
		t.Fatalf("正文仍应走 stdin，实际 %q", stdin)
	}
	if strings.Contains(cmd, "echo hi") {
		t.Fatalf("正文不该进命令行，实际 %q", cmd)
	}
	if !strings.Contains(cmd, ". /dev/stdin") {
		t.Fatalf("应让一个 source stdin 的 bash 去读 stdin（名字作 $0），实际 %q", cmd)
	}
	if !strings.Contains(cmd, "(1.2.3.4).sh") {
		t.Fatalf("脚本名应出现在下发行里，实际 %q", cmd)
	}
}

// python3 不补名字：`-c` 之后第一个参数落在 sys.argv[1] 而不是 argv[0]，补了只是多塞
// 一个脚本不认的参数。
func TestBuildCommandPythonKeepsNoScriptName(t *testing.T) {
	cmd, _ := BuildCommand("", "print(1)", "t.py", "python3", "", false, false, false)
	if strings.Contains(cmd, "t.py") {
		t.Fatalf("python3 不该补脚本名，实际 %q", cmd)
	}
}

// 带版本号的 shell 名（Debian 的 /bin/zsh5、ksh93）同样补名字：剥版本号的归一只有一份，
// 家族判定与 -a 支持范围、--no-shell 互斥认同一个名字（此前整名查表，zsh5 静默不补）。
func TestBuildCommandVersionedShellKeepsScriptName(t *testing.T) {
	for _, name := range []string{"zsh5", "ksh93", "/bin/bash5"} {
		cmd, _ := BuildCommand("", "echo hi", "t.sh", name, "", false, false, false)
		if !strings.Contains(cmd, ". /dev/stdin") || !strings.Contains(cmd, "t.sh") {
			t.Fatalf("%s 应补回脚本名（名字作 $0），实际 %q", name, cmd)
		}
	}
}

// zsh 的 source 会把 $0 重置成被 source 的文件名，内层串要先关 FUNCTION_ARGZERO
// 再 source，名字才保持 -c 参数给的值；bash / dash 无此行为，串里不出现 setopt。
func TestBuildCommandZshSourceKeepsArgZero(t *testing.T) {
	cmd, _ := BuildCommand("", "echo hi", "t.sh", "zsh5", "", false, false, false)
	if !strings.Contains(cmd, "setopt no_function_argzero; . /dev/stdin") {
		t.Fatalf("zsh 的内层串应先关 FUNCTION_ARGZERO 再 source，实际 %q", cmd)
	}
	if !strings.Contains(cmd, "t.sh") {
		t.Fatalf("zsh 也应补回脚本名，实际 %q", cmd)
	}
	for _, name := range []string{"bash", "dash", "ksh93"} {
		cmd, _ := BuildCommand("", "echo hi", "t.sh", name, "", false, false, false)
		if strings.Contains(cmd, "setopt") {
			t.Fatalf("%s 不该带 zsh 的 setopt，实际 %q", name, cmd)
		}
	}
}

// InterpreterBaseName：解释器名归一的唯一一份剥法，三个判断（-a 范围 / --no-shell
// 互斥 / $0 家族判定）都从这里认名字。
func TestInterpreterBaseName(t *testing.T) {
	cases := []struct{ in, want string }{
		{"bash", "bash"},
		{"/bin/bash", "bash"},
		{"BASH", "bash"},
		{"bash5", "bash"},
		{"/usr/bin/python3.11", "python"},
		{"php8.1", "php"},
		{"ksh93", "ksh"},
		{"perl -w", "perl"},
		{"", ""},
	}
	for _, c := range cases {
		if got := InterpreterBaseName(c.in); got != c.want {
			t.Fatalf("InterpreterBaseName(%q) 应为 %q，实际 %q", c.in, c.want, got)
		}
	}
}

// shellQuote：内部单引号按 '\” 转义，保证整串作为 sh -c 的单个参数传递。
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
// `sh -c '前缀; bash'`，故命令里的引号无需转义、也不会逃出参数边界。
// （早期实现走 base64 命令行通道，才有转义套娃；改用 stdin 后这条约束消失。）
func TestBuildCommandCommandTextNotOnCommandLine(t *testing.T) {
	cmd, stdin := BuildCommand(`echo 'hi'`, "", "", "bash", "", false, false, false)

	if strings.Contains(cmd, "hi") {
		t.Fatalf("命令原文不该出现在命令行里（已改走 stdin）：%q", cmd)
	}
	if stdin != `echo 'hi'` {
		t.Fatalf("命令原文应原样进 stdin，实际 %q", stdin)
	}
	// 命令行形态固定：sh -c '<单引号包裹的前缀; bash>'
	if !strings.HasPrefix(cmd, "sh -c '") || !strings.HasSuffix(cmd, "'") {
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

// DescribeCommand 命令模式：交代命令行发什么、stdin 送什么、内层是什么 shell。
//
// 不描述「包装前后」——内容走 stdin 通道后，日志里看不见内容本身，
// 所以只需把「实际交给 SSH 执行的事实」讲清：命令行、stdin、导入的环境变量。
func TestDescribeCommandCommandMode(t *testing.T) {
	const env = "export LC_ALL=C;"
	text := DescribeCommand(DescribeInput{Command: "who -b", Interpreter: "bash", EnvPrefix: env})

	for _, want := range []string{
		"交给 SSH 执行",
		"命令行： sh -c '" + env + " " + wantEncodingFragment + " bash'",
		"stdin：  who -b",
		"配置文件里的环境变量",
		"交给登录用户的 bash 执行", // 中文之间不留空格，西文两侧留
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("应有 %q，实际：\n%s", want, text)
		}
	}
	// 命令行必须与真下发的命令同源
	login, _ := BuildCommand("who -b", "", "", "bash", env, false, false, false)
	if !strings.Contains(text, login) {
		t.Fatalf("交代的命令行应与 BuildCommand 同源\n期望含：%s\n实际：\n%s", login, text)
	}
}

// sudo 身份：命令行里带 sudo，说明里点明是 root 身份的解释器。
func TestDescribeCommandSudo(t *testing.T) {
	text := DescribeCommand(DescribeInput{Command: "id -u", AsRoot: true, Interpreter: "bash"})

	login, _ := BuildCommand("id -u", "", "", "bash", "", false, true, false)
	if !strings.Contains(text, login) {
		t.Fatalf("sudo 时命令行应与 BuildCommand 同源\n期望含：%s\n实际：\n%s", login, text)
	}
	if !strings.Contains(text, "root 身份的 bash（sudo）") {
		t.Fatalf("应点明 root 身份，实际：\n%s", text)
	}
	// 中文之间不留空格：是 `（sudo）执行`，不是 `（sudo） 执行`
	if !strings.Contains(text, "（sudo）执行") {
		t.Fatalf("括号后不该多一个空格，实际：\n%s", text)
	}
}

// --no-shell：命令行就是命令原文，不出现外壳包装。
func TestDescribeCommandNoShell(t *testing.T) {
	text := DescribeCommand(DescribeInput{Command: "raw-cmd --flag", NoShell: true})

	if !strings.Contains(text, "命令行： raw-cmd --flag") {
		t.Fatalf("应把命令原文作为命令行交代，实际：\n%s", text)
	}
	if !strings.Contains(text, "--no-shell") {
		t.Fatalf("应说明是 --no-shell 形态，实际：\n%s", text)
	}
	if strings.Contains(text, "sh -c") {
		t.Fatalf("--no-shell 下不该出现外壳，实际：\n%s", text)
	}
}

// 脚本模式：命令行里换成解释器，stdin 报脚本路径（不打印正文）。
func TestDescribeCommandScriptMode(t *testing.T) {
	const env = "export LC_ALL=C;"
	text := DescribeCommand(DescribeInput{
		ScriptPath:  "/x/t.sh",
		ScriptBody:  "echo 1\necho 2",
		Interpreter: "bash",
		EnvPrefix:   env,
		AsRoot:      true,
	})

	for _, want := range []string{
		"交给 SSH 执行",
		"sh -c '" + env + " " + wantEncodingFragment + " sudo bash'",
		"stdin：  脚本 /x/t.sh 的内容（bash 解释）",
		"以 root 身份执行", // 中文之间不留空格
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("应有 %q，实际：\n%s", want, text)
		}
	}
	// 正文不进日志（脚本可能几百行）
	if strings.Contains(text, "echo 1") {
		t.Fatalf("脚本正文不该打印，实际：\n%s", text)
	}
}

// py 脚本用 python3 解释（命令行与说明两处都要跟着变）。
func TestDescribeCommandPythonScript(t *testing.T) {
	const env = "export LC_ALL=C;"
	text := DescribeCommand(DescribeInput{
		ScriptPath:  "/x/t.py",
		ScriptBody:  "print('hi')",
		Interpreter: "python3",
		EnvPrefix:   env,
	})
	if !strings.Contains(text, "; python3'") {
		t.Fatalf("命令行应换用 python3，实际：\n%s", text)
	}
	if !strings.Contains(text, "（python3 解释）") {
		t.Fatalf("说明应点明解释器，实际：\n%s", text)
	}
}

// 上传 / 下载模式没有命令可交代。
func TestDescribeCommandNoCommand(t *testing.T) {
	if text := DescribeCommand(DescribeInput{}); text != "" {
		t.Fatalf("无命令时应给空串，实际 %q", text)
	}
}

// ProgramTextArg：「程序文本参数」按解释器取，表外不支持；名字可带目录与版本号。
func TestProgramTextArg(t *testing.T) {
	cases := []struct {
		in   string
		want string
		ok   bool
	}{
		{"bash", "-c", true},
		{"/usr/bin/python3.11", "-c", true},
		{"perl", "-e", true},
		{"ruby", "-e", true},
		{"node", "-e", true},
		{"lua5.4", "-e", true},
		{"php8.1", "-r", true},
		{"tclsh", "", false},
		{"", "", false},
	}
	for _, c := range cases {
		got, ok := ProgramTextArg(c.in)
		if got != c.want || ok != c.ok {
			t.Fatalf("%q：应 (%q,%v)，实际 (%q,%v)", c.in, c.want, c.ok, got, ok)
		}
	}
}

// 程序文本参数不能写死 -c：perl 这类要用 -e 启动。
func TestInteractiveCommandUsesProgramArg(t *testing.T) {
	got := InteractiveCommand(DownlinkSpec{Command: "print 1", Interpreter: "perl"})
	if !strings.Contains(got, "perl -e ") {
		t.Fatalf("perl 应用 -e 启动程序文本，实际：%s", got)
	}
}

// 反例禁用词：这几种形态一条都不许出现——`sudo env` / `sudo -E` 改外壳形态，
// `bash -c 'export` 把环境塞进正文。两条都在 ADR-0030 里被否掉，出现即回退。
func TestDownlinkForbiddenForms(t *testing.T) {
	const env = `export PATH="/bin";`
	forms := map[string]string{
		"命令模式": mustCommand(t, "ls", "", "", "bash", env, false, false),
		"sudo":  mustCommand(t, "ls", "", "", "bash", env, false, true),
		"脚本模式": mustCommand(t, "", "echo hi", "t.sh", "bash", env, false, false),
		"代填": InteractiveCommand(DownlinkSpec{
			Command: "ls", Interpreter: "bash", EnvPrefix: env, AsRoot: true,
		}),
	}
	for what, got := range forms {
		for _, bad := range []string{"sudo env", "sudo -E", "bash -c 'export"} {
			if strings.Contains(got, bad) {
				t.Fatalf("%s 出现了禁用形态 %q：%s", what, bad, got)
			}
		}
	}
}
