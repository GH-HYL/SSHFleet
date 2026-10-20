package verdict

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sshfleet/internal/config"
)

// writeKeywordsConf 写一份临时判据表，返回路径。
func writeKeywordsConf(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "keywords_error.conf")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// 分类的两种合法形态：有关键词（查表得来），或标明了由程序定性（名字由代码给，配置只留名字与提示语）。
// 两者都没有，这条分类就永远匹配不上——启动时就该报出来，而不是等使用者看到一堆"未归类"。
func TestLoadKeywordsRejectsCategoryThatCanNeverMatch(t *testing.T) {
	body := "[[categories]]\nname = '远端无响应'\ntip = '多半卡在读写密码文件'\n"
	_, err := LoadKeywords(writeKeywordsConf(t, body))
	if err == nil {
		t.Fatal("有名字、没关键词、又没标明由程序定性的分类，应当加载失败")
	}
	if !strings.Contains(err.Error(), "远端无响应") {
		t.Fatalf("报错应当指出是哪个分类，实际：%v", err)
	}
}

// 第二组（退出码归类用）同一套校验。
func TestLoadKeywordsRejectsExitCodeCategoryThatCanNeverMatch(t *testing.T) {
	body := "[[categories]]\nname = '连接超时'\nkeywords = ['timed out']\n\n" +
		"[[exit_code_categories]]\nname = '仅允许 SFTP'\n"
	_, err := LoadKeywords(writeKeywordsConf(t, body))
	if err == nil {
		t.Fatal("退出码分块里同样不该放过匹配不上的分类")
	}
	if !strings.Contains(err.Error(), "仅允许 SFTP") {
		t.Fatalf("报错应当指出是哪个分类，实际：%v", err)
	}
}

// 标明由程序定性的分类，名字是代码给的、没有可匹配的文案，加载应当通过。
func TestLoadKeywordsAcceptsCodeDefinedCategory(t *testing.T) {
	body := "[[categories]]\nname = '远端无响应'\ntip = '多半卡在读写密码文件'\ncode_defined = true\n"
	if _, err := LoadKeywords(writeKeywordsConf(t, body)); err != nil {
		t.Fatalf("标明由程序定性的分类应加载通过：%v", err)
	}
}

// 由程序定性（不查表）的分类，名字在代码里、登记在配置里。两边任何一边漏了，
// 该分类都会在统计块落到「未归类」——或者反过来，配置里登记得挺全、代码里早已改名。
// 这里把「该由哪份配置登记」这份约定与随程序发布的两份真实配置逐条对账。
func TestCodeDefinedCategoriesAreDeclaredInConfig(t *testing.T) {
	errKw, err := LoadKeywords(filepath.Join("..", "..", "config", "keywords_error.conf"))
	if err != nil {
		t.Fatalf("通用判据表应可加载：%v", err)
	}
	pc, err := config.LoadPasswdConfig(filepath.Join("..", "..", "config", "keywords_passwd.conf"))
	if err != nil {
		t.Fatalf("改密配置应可加载：%v", err)
	}
	// 与 main 同一条装法：改密分类插进判据表最前面
	kw := &Keywords{}
	kw.PrependCategories(errKw.items)
	cats := make([]Category, 0, len(pc.Categories))
	for _, x := range pc.Categories {
		cats = append(cats, Category{Name: x.Name, Keywords: x.Keywords, Tip: x.Tip, CodeDefined: x.CodeDefined})
	}
	kw.PrependCategories(cats)

	for _, e := range codeDefinedNames {
		if !kw.Has(e.name) {
			t.Errorf("分类 %q 由程序定性，但两份配置里都没有登记它——上屏会落到「未归类」", e.name)
			continue
		}
		if e.config == "passwd" && !hasPasswdTip(pc, e.name) {
			t.Errorf("分类 %q 的提示语应在改密配置里（实际取到的是 %q）", e.name, kw.TipOf(e.name))
		}
	}
}

func hasPasswdTip(pc *config.PasswdConfig, name string) bool {
	for _, c := range pc.Categories {
		if c.Name == name && c.Tip != "" {
			return true
		}
	}
	return false
}

// codeDefinedNames 「由程序定性（不查表）」的分类名，以及该由哪份配置登记它们
// （error = 通用判据表，passwd = 改密配置）。
//
// 这份表**不是**被测代码的副本：被测的是"配置里登记了没有"，判据来自随程序发布的
// 配置文件；这里只是"应该登记哪些"的期望值。名字引用判定包的常量，改名会编译不过。
// 新增一个定性分类时，除了往这里加一行，还要在对应配置里登记同一名字——漏了测试会红。
var codeDefinedNames = []struct {
	name   string
	config string
}{
	{CategoryPasswordExpired, "error"},
	{CategoryPartialSuccess, "error"},
	{CategoryTriggerMiss, "error"},
	{CategoryExhaustedTimeout, "error"},
	{CategoryCanceled, "error"},
	{CategoryConnectCanceled, "error"},
	{CategoryNotExpired, "passwd"},
	{CategoryNoPrompt, "passwd"},
	{CategoryNoSignal, "passwd"},
	{CategoryNoReply, "passwd"},
	{CategoryGiveUp, "passwd"},
}
