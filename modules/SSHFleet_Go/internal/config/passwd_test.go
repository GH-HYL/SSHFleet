package config

import (
	"os"
	"strings"
	"testing"
)

func writePasswdConf(t *testing.T, body string) string {
	t.Helper()
	path := t.TempDir() + "/passwd.conf"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// 提示词表与失败分类装在一个文件里，一次读齐：两类各自校验，缺字段即报错。
func TestLoadPasswdConfig(t *testing.T) {
	ok := writePasswdConf(t, "[[step]]\nkeywords = ['新的密码']\nvalue = 'new'\nmax = 2\n\n"+
		"[[category]]\nname = '新密码不合规'\nkeywords = ['BAD PASSWORD']\ntip = 'x'\n")
	file, err := LoadPasswdConfig(ok)
	if err != nil {
		t.Fatalf("应加载成功：%v", err)
	}
	if len(file.Steps) != 1 || file.Steps[0].Max != 2 || file.Steps[0].Value != "new" {
		t.Fatalf("step 解析结果不对：%+v", file.Steps)
	}
	if len(file.Categories) != 1 || file.Categories[0].Name != "新密码不合规" {
		t.Fatalf("category 解析结果不对：%+v", file.Categories)
	}

	bad := map[string]string{
		"一条 step 都没有":         ``,
		"缺 keywords":          "[[step]]\nvalue = 'new'\nmax = 1\n",
		"value 写错":            "[[step]]\nkeywords = ['x']\nvalue = 'newest'\nmax = 1\n",
		"max 为 0":             "[[step]]\nkeywords = ['x']\nvalue = 'new'\nmax = 0\n",
		"keywords 里有空项":       "[[step]]\nkeywords = ['x', ' ']\nvalue = 'new'\nmax = 1\n",
		"多写了个不认识的字段":          "[[step]]\nkeywords = ['x']\nvalue = 'new'\nmax = 1\nwat = 1\n",
		"category 缺 name":     "[[step]]\nkeywords = ['x']\nvalue = 'new'\nmax = 1\n\n[[category]]\nkeywords = ['y']\n",
		"category 缺 keywords": "[[step]]\nkeywords = ['x']\nvalue = 'new'\nmax = 1\n\n[[category]]\nname = '甲'\n",
	}
	for name, body := range bad {
		if _, err := LoadPasswdConfig(writePasswdConf(t, body)); err == nil {
			t.Fatalf("%s 时应报错", name)
		}
	}

	// 失败分类可以为空：空即改密失败一律落回通用判据表
	if _, err := LoadPasswdConfig(writePasswdConf(t, "[[step]]\nkeywords = ['x']\nvalue = 'new'\nmax = 1\n")); err != nil {
		t.Fatalf("没有 category 时应能加载：%v", err)
	}
}

// 报错文案要指得出是哪一条。
func TestLoadPasswdConfigNamesTheEntry(t *testing.T) {
	body := "[[step]]\nkeywords = ['x']\nvalue = 'new'\nmax = 1\n\n" +
		"[[step]]\nkeywords = ['y']\nvalue = 'oops'\nmax = 1\n"
	if _, err := LoadPasswdConfig(writePasswdConf(t, body)); err == nil || !strings.Contains(err.Error(), "第 2 条") {
		t.Fatalf("应报出第 2 条，实际：%v", err)
	}
}
