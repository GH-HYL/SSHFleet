package parse

// 下发行的拼装：四段怎么拼、什么时候省哪一段，逐条对照老版。
//
// 命令模式那四条期望串是**老版二进制实际打出来的那一行**（`history/<run>/SSHFleetExec.log`
// 里的「命令行：」），不是照源码重写一遍——照源码重写只能证明两份代码像，不能证明发出的串
// 一样。取证现场：`build/建站04_10_2026-11-02_1245/ab/`，配置用 `SSHFleet-for-linux.conf`
// （`[env]` 是出厂那两条、`[locale] english = true`）。
//
// 脚本与 `-a` 两支的期望串按老版 `BuildCommand`／`InteractiveCommand` 的形态逐字写死：
// 正文 base64 进来之后整条串是确定的，可手算（见各用例里的注释）。

import (
	"encoding/base64"
	"strings"
	"testing"

	"sshfleet2/internal/contract"
	"sshfleet2/internal/message"
)

// factoryEnv 是出厂配置里那一段 `[env]`，黄金串照它取证。
var factoryEnv = map[string]string{
	"PATH": "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
	"TERM": "dumb",
}

// TestDownCommandMatchesLegacy 命令模式四种组合逐条对老版实际打出来的那一行。
func TestDownCommandMatchesLegacy(t *testing.T) {
	frag := encodingFragment
	exportFactory := `export PATH="/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin" TERM="dumb";`

	cases := []struct {
		name   string
		text   string
		run    contract.RunParams
		want   string
		wantIn string
	}{
		{
			// 老版：`-c uptime`（配置 sudo = true、english = true）
			name:   "root 身份 + 出厂环境段 + 提示语英文",
			text:   "uptime",
			run:    contract.RunParams{Interpreter: "bash", Env: factoryEnv, Sudo: true, English: true},
			want:   "sh -c '" + exportFactory + " " + frag + " export LC_MESSAGES=C; sudo bash'",
			wantIn: "uptime",
		},
		{
			// 老版：`-c uptime --no-sudo`——只差内层那个 sudo
			name:   "登录用户：不加 sudo",
			text:   "uptime",
			run:    contract.RunParams{Interpreter: "bash", Env: factoryEnv, English: true},
			want:   "sh -c '" + exportFactory + " " + frag + " export LC_MESSAGES=C; bash'",
			wantIn: "uptime",
		},
		{
			// 老版：`-c uptime --no-shell`——原样下发，不套外壳、不设环境、不加 sudo
			name:   "no-shell：原样下发",
			text:   "uptime",
			run:    contract.RunParams{Interpreter: "bash", Env: factoryEnv, Sudo: true, NoShell: true, English: true},
			want:   "uptime",
			wantIn: "",
		},
		{
			name:   "没配环境变量：只剩 locale 片段，不留多余空格",
			text:   "who -b",
			run:    contract.RunParams{Interpreter: "bash", English: true},
			want:   "sh -c '" + frag + " export LC_MESSAGES=C; bash'",
			wantIn: "who -b",
		},
		{
			name:   "english 关：不拼 LC_MESSAGES 那一句",
			text:   "who -b",
			run:    contract.RunParams{Interpreter: "bash", English: false},
			want:   "sh -c '" + frag + " bash'",
			wantIn: "who -b",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			plan := &contract.Plan{
				Mode: contract.ModeCommand,
				Task: contract.Task{Command: &contract.CommandTask{Text: c.text}},
				Run:  c.run,
			}
			got, stdin := downlinkOf(plan)
			if got != c.want {
				t.Fatalf("下发行不对\n期望：%s\n实际：%s", c.want, got)
			}
			if stdin != c.wantIn {
				t.Fatalf("stdin 应是 %q，实际 %q", c.wantIn, stdin)
			}
		})
	}
}

// TestDownlinkScriptGoesThroughStdin 脚本模式：正文走 stdin，shell 家族补 $0。
//
// 下发行里只有 `解释器 -c '. /dev/stdin' '脚本名'`——正文一个字都不在命令行上。
func TestDownlinkScriptGoesThroughStdin(t *testing.T) {
	plan := &contract.Plan{
		Mode: contract.ModeScript,
		Task: contract.Task{Script: &contract.ScriptTask{Path: "/tmp/deploy.sh", Body: "echo hi"}},
		Run:  contract.RunParams{Interpreter: "bash", Sudo: true},
	}
	got, stdin := downlinkOf(plan)
	// 内层那几个单引号会被外壳那对单引号按 '\'' 转义——老版拼出来的就是这一条。
	want := "sh -c " + shellQuote(encodingFragment+" sudo bash -c "+
		shellQuote(". /dev/stdin")+" "+shellQuote("deploy.sh"))
	if got != want {
		t.Fatalf("脚本模式的下发行不对\n期望：%s\n实际：%s", want, got)
	}
	if stdin != "echo hi" {
		t.Fatalf("脚本正文应当走 stdin，实际 %q", stdin)
	}
}

// zsh 的 source 会把 $0 重置成被 source 的文件名，所以它那段程序文本先关掉 FUNCTION_ARGZERO。
func TestDownlinkScriptZshTurnsOffFunctionArgzero(t *testing.T) {
	plan := &contract.Plan{
		Mode: contract.ModeScript,
		Task: contract.Task{Script: &contract.ScriptTask{Path: "/tmp/a.sh", Body: "echo"}},
		Run:  contract.RunParams{Interpreter: "zsh"},
	}
	got, _ := downlinkOf(plan)
	if !strings.Contains(got, "setopt no_function_argzero; . /dev/stdin") {
		t.Fatalf("zsh 那一段没关 FUNCTION_ARGZERO：%s", got)
	}
}

// TestDownlinkScriptPythonKeepsBareInterpreter python 不补 $0：`-c` 之后那个位置是
// sys.argv[1]，补了只是塞一个它不认的东西。
func TestDownlinkScriptPythonKeepsBareInterpreter(t *testing.T) {
	plan := &contract.Plan{
		Mode: contract.ModeScript,
		Task: contract.Task{Script: &contract.ScriptTask{Path: "/tmp/a.py", Body: "print(1)"}},
		Run:  contract.RunParams{Interpreter: "python3"},
	}
	got, _ := downlinkOf(plan)
	if !strings.HasSuffix(got, " python3'") {
		t.Fatalf("python 应当是裸解释器收尾：%s", got)
	}
}

// TestDownlinkAnswerEncodesBodyIntoCommandLine `-a`：正文 base64 编进命令行，
// stdin 让给代填（返回空串）。
func TestDownlinkAnswerEncodesBodyIntoCommandLine(t *testing.T) {
	frag := encodingFragment
	plan := &contract.Plan{
		Mode: contract.ModeCommand,
		Task: contract.Task{Command: &contract.CommandTask{Text: "install.sh"}},
		Run:  contract.RunParams{Interpreter: "bash", Answer: "y,是否继续"},
	}
	got, stdin := downlinkOf(plan)
	encoded := base64.StdEncoding.EncodeToString([]byte("install.sh"))
	want := "sh -c '" + frag + ` bash -c "$(printf %s ` + encoded + ` | base64 -d)"'`
	if got != want {
		t.Fatalf("-a 的下发行不对\n期望：%s\n实际：%s", want, got)
	}
	if stdin != "" {
		t.Fatalf("给了 -a 时 stdin 应当整条让给代填，实际 %q", stdin)
	}
}

// 脚本 + `-a`：正文编进命令行，脚本名仍作参数补回去（$0 才还原）。
func TestDownlinkAnswerWithScriptKeepsName(t *testing.T) {
	plan := &contract.Plan{
		Mode: contract.ModeScript,
		Task: contract.Task{Script: &contract.ScriptTask{Path: "/opt/deploy.sh", Body: "echo"}},
		Run:  contract.RunParams{Interpreter: "sh", Answer: "y,继续"},
	}
	got, _ := downlinkOf(plan)
	encoded := base64.StdEncoding.EncodeToString([]byte("echo"))
	// 内层单引号被外层那对转义过一次，这里按转义后的形态比。
	nameArg := strings.ReplaceAll(shellQuote("deploy.sh"), "'", `'\''`)
	if !strings.HasSuffix(got, `| base64 -d)" `+nameArg+`'`) {
		t.Fatalf("脚本名没补回 $0：%s", got)
	}
	if !strings.Contains(got, encoded) {
		t.Fatalf("正文没编进命令行：%s", got)
	}
}

// TestExportPrefixSortsAndQuotes 键名排序、值一律双引号、值里的双引号转义。
func TestExportPrefixSortsAndQuotes(t *testing.T) {
	env := map[string]string{
		"TERM": "dumb",
		"PATH": "/usr/bin",
		"QUOT": `say "hi"`,
	}
	want := `export PATH="/usr/bin" QUOT="say \"hi\"" TERM="dumb";`
	if got := exportPrefix(env); got != want {
		t.Fatalf("环境段不对\n期望：%s\n实际：%s", want, got)
	}
	if got := exportPrefix(nil); got != "" {
		t.Fatalf("空配置应给空串，实际 %q", got)
	}
}

// TestEncodingFragmentHasNoSingleQuote 片段里不许出现单引号。
//
// 理由：整段内层命令要被 shellQuote 包进一对单引号，片段自己再带单引号就得转义，
// 转义一多，量的串与发出去的串就不容易对上了。老版把这条写成不变式，这里守住它。
func TestEncodingFragmentHasNoSingleQuote(t *testing.T) {
	if strings.ContainsRune(encodingFragment, '\'') {
		t.Fatalf("编码判断片段里出现了单引号：%s", encodingFragment)
	}
}

// TestShellQuoteEscapesSingleQuote 内层有单引号时按 '\” 转义，整段仍是一个参数。
func TestShellQuoteEscapesSingleQuote(t *testing.T) {
	if got := shellQuote("a'b"); got != `'a'\''b'` {
		t.Fatalf("转义不对：%q", got)
	}
}

// TestInterpreterTableMatchesMessageNames 表里的名字与 message 那份显示名单不许漂。
//
// 两处各有一份名字是规格定的（表只放机器要的，「目前支持：…」那句与顺序归 message），
// 但名字本身必须一一对上——抄漏一个，使用者会在报错里看到工具其实不支持的名字。
func TestInterpreterTableMatchesMessageNames(t *testing.T) {
	got := map[string]bool{}
	for _, n := range interpreterNames() {
		got[n] = true
	}
	want := map[string]bool{}
	for _, n := range message.SupportedInterpreters() {
		want[n] = true
	}
	for n := range want {
		if !got[n] {
			t.Errorf("显示名单里有 %q，表里没有", n)
		}
	}
	for n := range got {
		if !want[n] {
			t.Errorf("表里有 %q，显示名单里没有", n)
		}
	}
}
