package execute

// 下发行的拼装：四段怎么拼、什么时候省哪一段，逐条对照老版。
//
// 期望串是**老版二进制实际打出来的那一行**（`history/<run>/SSHFleetExec.log` 里的
// 「命令行：」），不是照源码重写一遍——照源码重写只能证明两份代码像，不能证明发出的串一样。
// 取证现场：`build/建站04_10_2026-11-02_1245/ab/`，配置用 `SSHFleet-for-linux.conf`
// （`[env]` 是出厂那两条、`[locale] english = true`）。
//
// 量的串＝发的串＝日志里那一行，拼法一变就该在这几条上红出来。

import (
	"strings"
	"testing"

	"sshfleet2/internal/contract"
)

// factoryEnv 是出厂配置里那一段 `[env]`，黄金串照它取证。
var factoryEnv = map[string]string{
	"PATH": "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
	"TERM": "dumb",
}

// TestDownCommandMatchesLegacy 四种组合逐条对老版实际打出来的那一行。
func TestDownCommandMatchesLegacy(t *testing.T) {
	frag := encodingFragment
	exportFactory := `export PATH="/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin" TERM="dumb";`

	cases := []struct {
		name   string
		task   contract.CommandTask
		run    contract.RunParams
		want   string
		wantIn string
	}{
		{
			// 老版：`-c uptime`（配置 sudo = true、english = true）
			name:   "root 身份 + 出厂环境段 + 提示语英文",
			task:   contract.CommandTask{Text: "uptime"},
			run:    contract.RunParams{Interpreter: "bash", Env: factoryEnv, Sudo: true, English: true},
			want:   "sh -c '" + exportFactory + " " + frag + " export LC_MESSAGES=C; sudo bash'",
			wantIn: "uptime",
		},
		{
			// 老版：`-c uptime --no-sudo`——只差内层那个 sudo
			name:   "登录用户：不加 sudo",
			task:   contract.CommandTask{Text: "uptime"},
			run:    contract.RunParams{Interpreter: "bash", Env: factoryEnv, English: true},
			want:   "sh -c '" + exportFactory + " " + frag + " export LC_MESSAGES=C; bash'",
			wantIn: "uptime",
		},
		{
			// 老版：`-c uptime --no-shell`——原样下发，不套外壳、不设环境、不加 sudo
			name:   "no-shell：原样下发",
			task:   contract.CommandTask{Text: "uptime"},
			run:    contract.RunParams{Interpreter: "bash", Env: factoryEnv, Sudo: true, NoShell: true, English: true},
			want:   "uptime",
			wantIn: "",
		},
		{
			name:   "没配环境变量：只剩 locale 片段，不留多余空格",
			task:   contract.CommandTask{Text: "who -b"},
			run:    contract.RunParams{Interpreter: "bash", English: true},
			want:   "sh -c '" + frag + " export LC_MESSAGES=C; bash'",
			wantIn: "who -b",
		},
		{
			name:   "english 关：不拼 LC_MESSAGES 那一句",
			task:   contract.CommandTask{Text: "who -b"},
			run:    contract.RunParams{Interpreter: "bash", English: false},
			want:   "sh -c '" + frag + " bash'",
			wantIn: "who -b",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, stdin := downCommand(c.task, c.run)
			if got != c.want {
				t.Fatalf("下发行不对\n期望：%s\n实际：%s", c.want, got)
			}
			if stdin != c.wantIn {
				t.Fatalf("stdin 应是 %q，实际 %q", c.wantIn, stdin)
			}
		})
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
