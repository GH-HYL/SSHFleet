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

// 一条分类只要写了 name 就合法：没有关键词就是永远匹配不上、结果落到「未归类」，
// 由使用者自己负责。
func TestLoadKeywordsAcceptsCategoryWithoutKeywords(t *testing.T) {
	body := "[[categories]]\nname = '远端无响应'\ntip = '多半卡在读写密码文件'\n"
	if _, err := LoadKeywords(writeKeywordsConf(t, body)); err != nil {
		t.Fatalf("只写 name 的分类应加载通过：%v", err)
	}
}

// 第二组同理。
func TestLoadKeywordsAcceptsExitCodeCategoryWithoutKeywords(t *testing.T) {
	body := "[[categories]]\nname = '连接超时'\nkeywords = ['timed out']\n\n" +
		"[[exit_code_categories]]\nname = '仅允许 SFTP'\n"
	if _, err := LoadKeywords(writeKeywordsConf(t, body)); err != nil {
		t.Fatalf("退出码分块里只写 name 的分类也应加载通过：%v", err)
	}
}

// 缺 name 仍然报错，并指出是第几类。
func TestLoadKeywordsRejectsCategoryWithoutName(t *testing.T) {
	body := "[[categories]]\ntip = '没有名字'\nkeywords = ['x']\n"
	_, err := LoadKeywords(writeKeywordsConf(t, body))
	if err == nil {
		t.Fatal("缺 name 的分类应当加载失败")
	}
	if !strings.Contains(err.Error(), "缺少 name") {
		t.Fatalf("报错应当指出缺 name，实际：%v", err)
	}
}

// 配置里写了不认识的字段就报错，不许装作没事继续跑——否则使用者会以为那个字段还有用。
func TestLoadKeywordsRejectsUnknownField(t *testing.T) {
	body := "[[categories]]\nname = '远端无响应'\nunknown_field = true\n"
	_, err := LoadKeywords(writeKeywordsConf(t, body))
	if err == nil {
		t.Fatal("不认识的字段应当按未识别字段报错")
	}
	if !strings.Contains(err.Error(), "未识别字段") {
		t.Fatalf("报错应当指出是未识别字段，实际：%v", err)
	}
}

// 报文可以当关键词用：一条分类挂上 `sshfleet.transfer.rolled_back`，含这句话的文本就归它。
func TestMessageCanBeAKeyword(t *testing.T) {
	body := "[[categories]]\nname = '上传回滚失败'\nkeywords = ['sshfleet.transfer.rollback_partial']\n"
	kw, err := LoadKeywords(writeKeywordsConf(t, body))
	if err != nil {
		t.Fatal(err)
	}
	text := "a.log: 上传失败 - sshfleet.transfer.remote_file_failed: disk quota exceeded\n" +
		"sshfleet.transfer.rollback_partial: 1 of 2 written files removed, left: /data/b.bin"
	if hit := kw.matchText(false, text); hit != "上传回滚失败" {
		t.Fatalf("报文当关键词应命中「上传回滚失败」，实为 %q", hit)
	}
}

// 环境前缀可以认领一整摊：`sshfleet.transfer.` 一条认下全部传输报文。
func TestMessagePrefixClaimsWholeEnvironment(t *testing.T) {
	body := "[[categories]]\nname = '传输没成'\nkeywords = ['sshfleet.transfer.']\n"
	kw, err := LoadKeywords(writeKeywordsConf(t, body))
	if err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{
		"sshfleet.transfer.client_failed: ssh: subsystem request failed",
		"sshfleet.transfer.remote_missing: remote path not found: /data",
		"sshfleet.transfer.rolled_back: 6 of 6 written files removed",
	} {
		if hit := kw.matchText(false, text); hit != "传输没成" {
			t.Fatalf("%q 应被前缀认领，实为 %q", text, hit)
		}
	}
	// 别的环境不受牵连
	if hit := kw.matchText(false, "sshfleet.passwd.no_reply: password fed but no reply before timeout"); hit != "" {
		t.Fatalf("别的环境不该被传输前缀认领，实为 %q", hit)
	}
}

// 随程序发布的两份配置：报文时代的每一摊都要有分类认领它。
//
// 判的是"配置里有没有那种关键词"，不引用被测代码里的任何常量——期望值来自
// spec《预算：全部报文清单》判给各分类的那几条。
func TestShippedConfigsClaimEveryMessageFamily(t *testing.T) {
	errKw, err := LoadKeywords(filepath.Join("..", "..", "config", "keywords_error.conf"))
	if err != nil {
		t.Fatalf("通用判据表应可加载：%v", err)
	}
	pc, err := config.LoadPasswdConfig(filepath.Join("..", "..", "config", "keywords_passwd.conf"))
	if err != nil {
		t.Fatalf("改密配置应可加载：%v", err)
	}
	cats := make([]Category, 0, len(pc.Categories))
	for _, x := range pc.Categories {
		cats = append(cats, Category{Name: x.Name, Keywords: x.Keywords, Tip: x.Tip})
	}
	errKw.PrependCategories(cats)

	families := []struct {
		keyword string
		expect  string
	}{
		{"sshfleet.connect.refused", "拒绝网络连接"},
		{"sshfleet.connect.handshake_timeout", "握手超时"},
		{"sshfleet.connect.key_parse_failed", "密钥解析失败"},
		{"sshfleet.session.create_failed", "无法建立会话"},
		{"sshfleet.session.pty_failed", "无法建立会话"},
		{"sshfleet.session.exit_missing", "会话被中断"},
		{"sshfleet.run.timeout", "执行超时"},
		{"sshfleet.run.rejected", "命令被拒绝"},
		{"sshfleet.session.channel_failed", "无法建立会话"},
		{"sshfleet.transfer.client_failed", "目标机不支持 SFTP"},
		{"sshfleet.transfer.remote_file_failed", "目标机写入失败"},
		{"sshfleet.transfer.tmp_failed", "目标机写入失败"},
		{"sshfleet.transfer.local_file_failed", "本机写入失败"},
		{"sshfleet.transfer.remote_not_dir", "上传目标不是目录"},
		{"sshfleet.transfer.rolled_back", "上传失败已回滚"},
		{"sshfleet.transfer.rollback_partial", "上传回滚未清干净"},
		{"sshfleet.transfer.local_unreadable", "本地文件错误"},
		{"sshfleet.transfer.remote_missing", "远程路径不存在"},
		{"sshfleet.transfer.no_files", "远端没有文件"},
		{"sshfleet.abort.canceled", "任务已取消"},
		{"sshfleet.abort.connect_canceled", "建连被取消"},
		{"sshfleet.answer.missed", "触发词未命中"},
		{"sshfleet.passwd.no_prompt", "提示词没认出"},
		{"sshfleet.passwd.no_signal", "没等到提示"},
		{"sshfleet.passwd.no_reply", "远端无响应"},
		{"sshfleet.passwd.give_up", "改密未完成"},
		{"sshfleet.passwd.not_expired", "密码未过期"},
	}
	for _, f := range families {
		if !errKw.Has(f.expect) {
			t.Errorf("配置里没有分类 %q", f.expect)
			continue
		}
		if !hasKeyword(errKw, f.expect, f.keyword) {
			t.Errorf("分类 %q 没有认领报文 %q——那一摊失败会落到「未归类」", f.expect, f.keyword)
		}
	}
}

func hasKeyword(kw *Keywords, category, keyword string) bool {
	for _, k := range kw.KeywordsOf(category) {
		if k == keyword {
			return true
		}
	}
	return false
}

// 一条报文只该归一个分类：`answer.exhausted_timeout` 同时挂在两处时，
// 谁排在前面就永远赢——后面那条等于死条目。这里守住"每个报文关键词只出现一次"。
func TestMessageKeywordBelongsToOneCategory(t *testing.T) {
	errKw, err := LoadKeywords(filepath.Join("..", "..", "config", "keywords_error.conf"))
	if err != nil {
		t.Fatal(err)
	}
	pc, err := config.LoadPasswdConfig(filepath.Join("..", "..", "config", "keywords_passwd.conf"))
	if err != nil {
		t.Fatal(err)
	}
	var cats []Category
	cats = append(cats, errKw.items...)
	cats = append(cats, errKw.exitItems...)
	for _, x := range pc.Categories {
		cats = append(cats, Category{Name: x.Name, Keywords: x.Keywords})
	}

	owner := map[string]string{}
	for _, c := range cats {
		for _, kw := range c.Keywords {
			if !strings.HasPrefix(kw, "sshfleet.") {
				continue
			}
			if prev, ok := owner[kw]; ok && prev != c.Name {
				t.Errorf("报文关键词 %q 同时挂在 %q 与 %q：排在后面的那条永远认不到", kw, prev, c.Name)
			}
			owner[kw] = c.Name
		}
	}
	if len(owner) == 0 {
		t.Fatal("没扫到任何报文关键词，断言失效")
	}
}

// 「密码过期」这个分类名是**跨包约定**：批量改密的入场信号与代填的中止词都按名查表
// （`Keywords.PasswordExpired()`）。改它的 name 会让查表落空——启动期的守卫据此报错
// 退出，而不是每台机器静默干等到超时。这条用例锁定「改 name → 取到空表」这个事实。
func TestPasswordExpiredEmptyAfterRename(t *testing.T) {
	if got := LoadKeywordsForTest(t, "[[categories]]\nname = '密码过期'\nkeywords = ['WARNING: Your password has expired.']\n").PasswordExpired(); len(got) != 1 {
		t.Fatalf("原名下应取到 1 条入场信号关键词，实际 %v", got)
	}

	renamed := LoadKeywordsForTest(t,
		"[[categories]]\nname = '密码已到期'\nkeywords = ['WARNING: Your password has expired.']\n")
	if got := renamed.PasswordExpired(); len(got) != 0 {
		t.Fatalf("改掉 name 后入场信号应取空（守卫据此报错退出），实际 %v", got)
	}
}

// LoadKeywordsForTest 读临时判据表，出错即 Fatal。
func LoadKeywordsForTest(t *testing.T, body string) *Keywords {
	t.Helper()
	kw, err := LoadKeywords(writeKeywordsConf(t, body))
	if err != nil {
		t.Fatal(err)
	}
	return kw
}
