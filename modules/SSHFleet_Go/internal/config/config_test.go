package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"
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

// 加密关着时 secret_dir 没有用途，写成相对目录不该拦人（ADR-0025 决定 4）。
// 用随包发布的那份模板改两行，走真正的装载入口。
func TestLoadAllowsRelativeSecretDirWhenEncryptOff(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "config", "SSHFleet-for-linux.conf"))
	if err != nil {
		t.Fatalf("读模板失败：%v", err)
	}
	conf := strings.Replace(string(raw), `secret_dir   = "/opt/SSHFleet/keys"`, `secret_dir   = "keys"`, 1)
	conf = strings.Replace(conf, "encrypt      = true", "encrypt      = false", 1)
	path := filepath.Join(t.TempDir(), "SSHFleet.conf")
	if err := os.WriteFile(path, []byte(conf), 0o644); err != nil {
		t.Fatalf("写临时配置失败：%v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("加密关着、secret_dir 写相对路径时不该报错：%v", err)
	}
	if cfg.Credential.SecretDir != "keys" {
		t.Fatalf("关着加密时不该加工这个值，实际 %q", cfg.Credential.SecretDir)
	}
}

// [locale] english：出厂是 true（必填）；改成 false 能读到；类型写错报解码错；
// 整段删掉报必填字段缺失。三种都走既有的那两条口径，不额外加校验。
func TestLoadLocaleEnglish(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "config", "SSHFleet-for-linux.conf"))
	if err != nil {
		t.Fatalf("读模板失败：%v", err)
	}
	template := string(raw)

	load := func(t *testing.T, conf string) (*Config, error) {
		t.Helper()
		path := filepath.Join(t.TempDir(), "SSHFleet.conf")
		if err := os.WriteFile(path, []byte(conf), 0o644); err != nil {
			t.Fatalf("写临时配置失败：%v", err)
		}
		return Load(path)
	}

	cfg, err := load(t, template)
	if err != nil {
		t.Fatalf("出厂模板应能加载：%v", err)
	}
	if !cfg.Locale.English {
		t.Fatalf("出厂应是 true（提示语统一英文），实际 %v", cfg.Locale.English)
	}

	cfg, err = load(t, strings.Replace(template, "english = true", "english = false", 1))
	if err != nil {
		t.Fatalf("改成 false 应能加载：%v", err)
	}
	if cfg.Locale.English {
		t.Fatalf("english = false 应读成 false，实际 %v", cfg.Locale.English)
	}

	if _, err := load(t, strings.Replace(template, "english = true", `english = "yes"`, 1)); err == nil {
		t.Fatalf("english 写字符串应报错")
	}

	if _, err := load(t, dropSection(template, "locale")); err == nil ||
		!strings.Contains(err.Error(), "locale.english") {
		t.Fatalf("整段 [locale] 删掉应报缺少必填字段 locale.english，实际 %v", err)
	}
}

// dropSection 删掉配置里的整段（段头连同段内各行），用来验证「整段缺失」这一类口径。
func dropSection(conf, name string) string {
	lines := strings.Split(conf, "\n")
	out := make([]string, 0, len(lines))
	skipping := false
	for _, ln := range lines {
		trimmed := strings.TrimSpace(ln)
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			skipping = trimmed == "["+name+"]"
			if skipping {
				continue
			}
		}
		if skipping {
			continue
		}
		out = append(out, ln)
	}
	return strings.Join(out, "\n")
}

// 三份出厂模板是同一条约定的三份副本，打包按平台取其中一份改名成包内 config/SSHFleet.conf
// （tools/release/release.py）。所以内容约束要三份一起钉，只钉 linux 那份等于漏掉 windows 包。
//
// 钉的是三条：`[env]` 只有 PATH 与 TERM（编码与语言都不许再塞回来）、`[locale] english` 为 true。
func TestFactoryTemplatesContent(t *testing.T) {
	for _, name := range []string{"SSHFleet.conf", "SSHFleet-for-linux.conf", "SSHFleet-for-windows.conf"} {
		t.Run(name, func(t *testing.T) {
			var tpl struct {
				Env    map[string]string `toml:"env"`
				Locale struct {
					English bool `toml:"english"`
				} `toml:"locale"`
			}
			if _, err := toml.DecodeFile(filepath.Join("..", "..", "config", name), &tpl); err != nil {
				t.Fatalf("模板解不开：%v", err)
			}
			if len(tpl.Env) != 2 || tpl.Env["PATH"] == "" || tpl.Env["TERM"] != "dumb" {
				t.Fatalf("出厂 [env] 应正好是 PATH 与 TERM=dumb，实际 %v", tpl.Env)
			}
			for _, banned := range []string{"LANG", "LC_ALL", "LC_CTYPE", "LC_MESSAGES"} {
				if _, ok := tpl.Env[banned]; ok {
					t.Fatalf("出厂 [env] 不该再有 %s（编码与语言都不走 [env]）", banned)
				}
			}
			if !tpl.Locale.English {
				t.Fatalf("出厂 english 应为 true")
			}
		})
	}
}
