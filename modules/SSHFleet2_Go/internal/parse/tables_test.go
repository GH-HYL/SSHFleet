package parse

import (
	"os"
	"path/filepath"
	"testing"

	"sshfleet2/internal/contract"
)

// 判据表的编译口径（自定义区）：老版编译危险规则时统一加 `(?i)`（对位旧实现的
// re.IGNORECASE）。不照搬就会漏掉 `RM -RF /etc` 这一类大写写法——那是同一个命令。
func TestBuildRuleCompilesCaseInsensitive(t *testing.T) {
	rule, bad := buildRule(map[string]string{
		"name":       "递归删除系统目录",
		"regex":      `\brm\s+-rf\s+/etc(?:/\*)?(?:[\s;&|)]|$)`,
		"risk_level": "high",
	}, 1)
	if bad != nil {
		t.Fatalf("这条规则本身是合法的，不该报错：%+v", bad)
	}
	if len(rule.Patterns) != 1 {
		t.Fatalf("应当编译出一条正则，实际 %d 条", len(rule.Patterns))
	}
	for _, s := range []string{"rm -rf /etc", "RM -RF /ETC", "Rm -Rf /Etc "} {
		if !rule.Patterns[0].MatchString(s) {
			t.Fatalf("%q 应当命中", s)
		}
	}
}

// 分类名重名只查同一块内（自定义区）：`category` 与 `exit_code_category` 是两份互不相干的
// 表，跨块同名合法；同一块里两条同名才报，报的是第几条与第几条重名、那个名字是什么。
func TestBuildCategoriesDuplicateOnlyWithinBlock(t *testing.T) {
	const doc = `
[[category]]
name = '上传失败'
keywords = ["上传失败"]

[[category]]
name = '上传失败'
keywords = ["又一条"]

[[exit_code_category]]
name = '上传失败'
keywords = ["上传失败"]
`
	lines, err := tokenize([]byte(doc))
	if err != nil {
		t.Fatalf("这份文件是合法的 TOML 子集，不该切不动：%v", err)
	}
	// 两块各查一次：exit_code_category 只有一条，与 category 块同名不算错。
	if _, bad := buildCategories(lines, exitCategoriesKey); bad != nil {
		t.Fatalf("跨块同名是合法的，不该报：%+v", *bad)
	}
	_, bad := buildCategories(lines, errorCategoriesKey)
	if bad == nil {
		t.Fatalf("同一块里第 1 条与第 2 条重名，应当报")
	}
	if bad.Kind != contract.CannotStartKeywordCatDuplicate || bad.Index != 1 || bad.Limit != 2 || bad.Value != "上传失败" {
		t.Fatalf("重名那一格装错了：%+v", *bad)
	}
}

// 改密提示词表的 category 块同样只查块内重名（自定义区）：step 块没有 name 这一格，不参与。
//
// 判据表路径是随包写死的相对路径、没有注入点，所以这里用临时目录造一份现场再切过去
// （`t.Chdir` 会在用例结束时切回）。
func TestLoadPasswdKeywordsRejectsDuplicateCategory(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "config"), 0o755); err != nil {
		t.Fatalf("建临时配置目录失败：%v", err)
	}
	conf := filepath.Join(dir, "config", "keywords_passwd.conf")
	step := "[[step]]\n" +
		"keywords = [\"当前密码\"]\n" +
		"value = \"" + contract.PasswdValueCurrent + "\"\n" +
		"max = 1\n" +
		"\n"
	dup := step +
		"[[category]]\n" +
		"name = '改密成功'\n" +
		"keywords = [\"密码更新成功\"]\n" +
		"\n[[category]]\n" +
		"name = '改密成功'\n" +
		"keywords = [\"又一条\"]\n"
	if err := os.WriteFile(conf, []byte(dup), 0o644); err != nil {
		t.Fatalf("写临时判据表失败：%v", err)
	}
	t.Chdir(dir)

	_, _, bad := loadPasswdKeywords()
	if bad == nil {
		t.Fatalf("category 第 1 条与第 2 条重名，应当报")
	}
	if r := bad.Reason; r.Kind != contract.CannotStartPasswdCatDuplicate || r.Index != 1 || r.Limit != 2 || r.Place != "category" || r.Value != "改密成功" {
		t.Fatalf("改密重名那一格装错了：%+v", r)
	}

	// 换成两个不同名字，不该再报——防把「见到 category 就报」写成过度检查。
	clean := step +
		"[[category]]\n" +
		"name = '改密成功'\n" +
		"keywords = [\"密码更新成功\"]\n" +
		"\n[[category]]\n" +
		"name = '改密失败'\n" +
		"keywords = [\"密码未更改\"]\n"
	if err := os.WriteFile(conf, []byte(clean), 0o644); err != nil {
		t.Fatalf("改写临时判据表失败：%v", err)
	}
	if _, _, bad := loadPasswdKeywords(); bad != nil {
		t.Fatalf("两条名字不同，不该报重名：%+v", *bad)
	}
}
