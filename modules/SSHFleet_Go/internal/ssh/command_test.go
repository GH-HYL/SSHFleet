package ssh

import (
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
