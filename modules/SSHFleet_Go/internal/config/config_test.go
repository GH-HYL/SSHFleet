package config

import (
	"strings"
	"testing"
)

// validateEnv：键名必须是合法 shell 变量名（挡 123 / A-B）；空键与空值一律丢弃；
// 值不能以奇数个反斜杠结尾（会吃掉下发串的收尾引号）。
func TestValidateEnv(t *testing.T) {
	ok := map[string]string{"LC_ALL": "en_US.UTF-8", "PATH": "/bin:/usr/bin", "_X1": "v"}
	if err := validateEnv(ok); err != nil {
		t.Fatalf("正常配置不该报错：%v", err)
	}
	if len(ok) != 3 {
		t.Fatalf("合法条目不该被丢，实际 %v", ok)
	}

	// 空键 / 空值 = 没有这一条
	drop := map[string]string{"A": "1", "B": "", "": "x"}
	if err := validateEnv(drop); err != nil {
		t.Fatalf("空键空值应放行（按不存在处理）：%v", err)
	}
	if len(drop) != 1 || drop["A"] != "1" {
		t.Fatalf("空键空值应被丢弃，实际 %v", drop)
	}

	for _, bad := range []string{"123", "1A", "A-B", "A B", "A.B"} {
		if err := validateEnv(map[string]string{bad: "v"}); err == nil || !strings.Contains(err.Error(), "不是合法的环境变量名") {
			t.Fatalf("%q 应被拦下，实际 %v", bad, err)
		}
	}

	// 值中间的反斜杠、成对的反斜杠都放行；只有末尾落单的那个会破坏下发串
	for _, value := range []string{`C:\dir`, `a\\b`, `尾\\`} {
		if err := validateEnv(map[string]string{"P": value}); err != nil {
			t.Fatalf("%q 应放行，实际 %v", value, err)
		}
	}
	for _, value := range []string{`尾\`, `尾\\\`} {
		if err := validateEnv(map[string]string{"P": value}); err == nil || !strings.Contains(err.Error(), "不能以反斜杠结尾") {
			t.Fatalf("%q 应被拦下，实际 %v", value, err)
		}
	}
}

// validateInterpreter：命令项与脚本映射都必须"配了就有值"；后缀带点、不重复（忽略大小写）；
// 解释器重复放行；后缀键归一为小写。
func TestValidateInterpreter(t *testing.T) {
	if err := validateInterpreter(&Interpreter{Command: "bash", Script: map[string]string{".sh": "bash", ".py": "python3"}}); err != nil {
		t.Fatalf("正常配置不该报错：%v", err)
	}

	// 归一：后缀键变小写
	it := &Interpreter{Command: "bash", Script: map[string]string{".SH": "bash"}}
	if err := validateInterpreter(it); err != nil {
		t.Fatalf("大写后缀应放行：%v", err)
	}
	if _, ok := it.Script[".sh"]; !ok {
		t.Fatalf("后缀键应归一为小写，实际 %v", it.Script)
	}

	cases := []struct {
		name string
		it   *Interpreter
		want string
	}{
		{"命令项为空", &Interpreter{Script: map[string]string{".sh": "bash"}}, "interpreter.command"},
		{"脚本映射为空", &Interpreter{Command: "bash"}, "interpreter.script"},
		{"后缀为空", &Interpreter{Command: "bash", Script: map[string]string{"": "bash"}}, "后缀是空的"},
		{"后缀没带点", &Interpreter{Command: "bash", Script: map[string]string{"sh": "bash"}}, "要以点开头"},
		{"解释器为空", &Interpreter{Command: "bash", Script: map[string]string{".sh": " "}}, "没有配解释器"},
		{"后缀重复（忽略大小写）", &Interpreter{Command: "bash", Script: map[string]string{".sh": "bash", ".SH": "sh"}}, "后缀重复"},
		{"解释器重复放行", &Interpreter{Command: "bash", Script: map[string]string{".sh": "bash", ".bash": "bash"}}, ""},
	}
	for _, c := range cases {
		err := validateInterpreter(c.it)
		if c.want == "" {
			if err != nil {
				t.Fatalf("%s：不该报错，实际 %v", c.name, err)
			}
			continue
		}
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Fatalf("%s：应含 %q，实际 %v", c.name, c.want, err)
		}
	}
}
