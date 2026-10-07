package cli

import (
	"strings"
	"testing"

	"sshfleet/internal/common"
	"sshfleet/internal/config"
)

func testInterpCfg() *config.Config {
	return &config.Config{Interpreter: config.Interpreter{
		Command: "bash",
		Script:  map[string]string{".sh": "bash", ".py": "python3"},
	}}
}

// MatchScriptInterpreter：取最后一段扩展名、忽略大小写；没命中返回 false。
func TestMatchScriptInterpreter(t *testing.T) {
	table := map[string]string{".sh": "bash", ".py": "python3"}
	cases := []struct {
		path, want string
		ok         bool
	}{
		{"a.sh", "bash", true},
		{"a.tar.sh", "bash", true},
		{"A.SH", "bash", true},
		{"x.py", "python3", true},
		{"x.pl", "", false},
		{"noext", "", false},
	}
	for _, c := range cases {
		got, ok := MatchScriptInterpreter(c.path, table)
		if ok != c.ok || got != c.want {
			t.Fatalf("%s：应 (%q,%v)，实际 (%q,%v)", c.path, c.want, c.ok, got, ok)
		}
	}
}

// interpreterOf：-c 用命令解释器；-s 命中映射用它、没命中回退命令解释器；其余模式为空。
func TestInterpreterOf(t *testing.T) {
	cfg := testInterpCfg()
	if got := interpreterOf(&Args{Command: "ls"}, cfg); got != "bash" {
		t.Fatalf("-c 应用命令解释器，实际 %q", got)
	}
	if got := interpreterOf(&Args{Script: "a.py"}, cfg); got != "python3" {
		t.Fatalf("-s 命中映射应用映射值，实际 %q", got)
	}
	if got := interpreterOf(&Args{Script: "a.pl"}, cfg); got != "bash" {
		t.Fatalf("-s 没命中应回退命令解释器，实际 %q", got)
	}
	if got := interpreterOf(&Args{Upload: "x"}, cfg); got != "" {
		t.Fatalf("上传模式不该有解释器，实际 %q", got)
	}
}

// ConfirmInterpreterFallback：命中映射不问；没配到才问，答 n 取消、答 y 放行。
func TestConfirmInterpreterFallback(t *testing.T) {
	cfg := testInterpCfg()
	newIn := func(input string) (*common.Interactor, *strings.Builder) {
		out := &strings.Builder{}
		return &common.Interactor{In: strings.NewReader(input), Out: out}, out
	}

	// 命中映射：不问、无输出
	in, out := newIn("n\n")
	if err := ConfirmInterpreterFallback(&Args{Script: "a.py"}, cfg, in); err != nil {
		t.Fatalf("命中映射不该提问，实际 %v", err)
	}
	if out.Len() != 0 {
		t.Fatalf("命中映射不该有任何输出，实际 %q", out.String())
	}

	// 没命中 + 答 n：取消，且提示里带出回退的解释器
	in, out = newIn("n\n")
	if err := ConfirmInterpreterFallback(&Args{Script: "a.bin"}, cfg, in); err != common.ErrCancelled {
		t.Fatalf("答 n 应取消，实际 %v", err)
	}
	if !strings.Contains(out.String(), "bash") {
		t.Fatalf("提示要点明回退的解释器：%q", out.String())
	}

	// 没命中 + 答 y：放行
	in, _ = newIn("y\n")
	if err := ConfirmInterpreterFallback(&Args{Script: "a.bin"}, cfg, in); err != nil {
		t.Fatalf("答 y 应放行，实际 %v", err)
	}
}

// isBashInterpreter：--no-shell 与自定义解释器互斥的判断依据（bash 与 /bin/bash 都算）。
func TestIsBashInterpreter(t *testing.T) {
	for _, name := range []string{"bash", "/bin/bash", "BASH", "bash5"} {
		if !IsBashInterpreter(name) {
			t.Fatalf("%s 应判为 bash", name)
		}
	}
	for _, name := range []string{"sh", "dash", "zsh5", "perl", "python3", ""} {
		if IsBashInterpreter(name) {
			t.Fatalf("%s 不该判为 bash", name)
		}
	}
}

// --no-shell 与解释器配置一起用不再报错（2026-10-08 作者定）：no-shell 下解释器配置
// 本来就不参与执行，参数屏对非 bash 的配置提示一句即可，不该逼人改配置。
func TestCheckArgumentsNoShellWithInterpreter(t *testing.T) {
	if err := CheckArguments(&Args{Command: "ls", NoShell: true, Interpreter: "perl"}); err != nil {
		t.Fatalf("--no-shell + 非 bash 解释器不应报错（参数屏提示代替硬拦），实际：%v", err)
	}
}

// -s 与 --no-shell 互斥：no-shell 只对 -c 有意义，脚本没有"原样下发"的形态；
// 此前加在 -s 上会被静默丢掉、照常执行。
func TestCheckArgumentsNoShellVsScript(t *testing.T) {
	err := CheckArguments(&Args{Script: "x.sh", NoShell: true})
	if err == nil || !strings.Contains(err.Error(), "-s 不能和 --no-shell") {
		t.Fatalf("-s + --no-shell 应报互斥，实际：%v", err)
	}

	// 不带 --no-shell 的脚本不走这条互斥（后续脚本文件检查另算）
	err = CheckArguments(&Args{Script: "x.sh"})
	if err != nil && strings.Contains(err.Error(), "--no-shell") {
		t.Fatalf("不带 --no-shell 不该触发这条互斥：%v", err)
	}
}

// checkAnswerInterpreter：-a 只在解释器属于支持范围时放行（含带目录/版本号的写法），
// 范围外当场报错并点名；没用 -a 一律放行。
func TestCheckAnswerInterpreter(t *testing.T) {
	supported := []string{"bash", "sh", "/usr/bin/python3.11", "perl", "ruby", "node", "lua5.4", "php8.1"}
	for _, name := range supported {
		a := &Args{Command: "x", Answer: "1,架构", Interpreter: name}
		if err := checkAnswerInterpreter(a); err != nil {
			t.Fatalf("%s 在支持范围内，不该报错：%v", name, err)
		}
	}

	// 表外：报错、点名、并把支持范围与去处讲清
	a := &Args{Command: "x", Answer: "1,架构", Interpreter: "tclsh"}
	err := checkAnswerInterpreter(a)
	if err == nil || !strings.Contains(err.Error(), "tclsh") || !strings.Contains(err.Error(), "不支持") {
		t.Fatalf("表外解释器应报不支持并点名，实际：%v", err)
	}

	// 没用 -a：与解释器无关，一律放行
	if err := checkAnswerInterpreter(&Args{Command: "x", Interpreter: "tclsh"}); err != nil {
		t.Fatalf("没用 -a 不该检查解释器：%v", err)
	}
}
