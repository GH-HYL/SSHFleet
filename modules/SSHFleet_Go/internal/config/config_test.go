package config

import (
	"strings"
	"testing"
)

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
