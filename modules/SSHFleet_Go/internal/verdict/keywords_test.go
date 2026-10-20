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
// 由使用者自己负责。这条规矩是报文体系定下的——原先为"程序定性"单开的 code_defined
// 通道已整体下线。
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

// code_defined 这个字段不再存在：配置里还写着它，按"未识别字段"报错——
// 旧配置不能装作没事继续跑，否则使用者会以为那个字段还有用。
func TestLoadKeywordsRejectsRetiredCodeDefinedField(t *testing.T) {
	body := "[[categories]]\nname = '远端无响应'\ncode_defined = true\n"
	_, err := LoadKeywords(writeKeywordsConf(t, body))
	if err == nil {
		t.Fatal("code_defined 已不存在，应当按未识别字段报错")
	}
	if !strings.Contains(err.Error(), "code_defined") {
		t.Fatalf("报错应当指出是 code_defined，实际：%v", err)
	}
}

// 报文可以当关键词用：一条分类挂上 `sshfleet.transfer.finished`，含这句话的文本就归它。
func TestMessageCanBeAKeyword(t *testing.T) {
	body := "[[categories]]\nname = '部分成功'\nkeywords = ['sshfleet.transfer.finished']\n"
	kw, err := LoadKeywords(writeKeywordsConf(t, body))
	if err != nil {
		t.Fatal(err)
	}
	text := "c.log: 下载失败 - permission denied\nsshfleet.transfer.finished: 3 of 5 files transferred"
	if hit := kw.matchText(false, text); hit != "部分成功" {
		t.Fatalf("报文当关键词应命中「部分成功」，实为 %q", hit)
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
		"sshfleet.transfer.finished: 6 of 10 files transferred",
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
		{"sshfleet.transfer.client_failed", "SFTP失败"},
		{"sshfleet.transfer.finished", "部分成功"},
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
