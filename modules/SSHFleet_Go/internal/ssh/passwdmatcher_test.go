package ssh

import (
	"os"
	"strings"
	"testing"
)

// 改密提示词表的匹配语义：入场信号作真门、一条可重复消费带上限、
// 多条并行取最先出现者。匹配口径（子串/正则、大小写）与代填共用 [interactive] 两个开关。

func testPrompts() *PasswdPrompts {
	return &PasswdPrompts{Steps: []PasswdStep{
		{Keywords: []string{"当前的密码", "Current password"}, Value: PasswdValueCurrent, Max: 1},
		{Keywords: []string{"新的密码", "New password"}, Value: PasswdValueNew, Max: 4},
		{Keywords: []string{"重新输入", "Retype"}, Value: PasswdValueNew, Max: 4},
	}}
}

var testEnterKeywords = []string{"password has expired", "密码已过期"}

func TestPasswdMatcherFeedsAfterEnterSignal(t *testing.T) {
	m := newPasswdMatcher(testPrompts(), testEnterKeywords, MatchOptions{})
	got := m.feed("WARNING: Your password has expired.\r\n当前的密码：")
	if len(got) != 1 || got[0] != PasswdValueCurrent {
		t.Fatalf("入场信号之后应先喂当前密码，实际：%v", got)
	}
	if !m.sawEnterSignal() {
		t.Fatal("应记成出现过入场信号")
	}
}

// 入场信号作真门：没出现之前，各条提示一律不参战（未过期的机器上本来也不会有这些提示）。
func TestPasswdMatcherGatedByEnterSignal(t *testing.T) {
	m := newPasswdMatcher(testPrompts(), testEnterKeywords, MatchOptions{})
	if got := m.feed("当前的密码：新的密码："); len(got) != 0 {
		t.Fatalf("入场信号之前不该喂任何东西，实际：%v", got)
	}
	if m.sawEnterSignal() {
		t.Fatal("不该记成出现过入场信号")
	}
}

// 可重复消费：同一个提示重问就再喂一次，最多 max 次。
func TestPasswdMatcherRepeatsUpToMax(t *testing.T) {
	m := newPasswdMatcher(testPrompts(), testEnterKeywords, MatchOptions{})
	m.feed("密码已过期")

	for i := 1; i <= 4; i++ {
		got := m.feed("新的密码：")
		if len(got) != 1 || got[0] != PasswdValueNew {
			t.Fatalf("第 %d 次重问应再喂新密码，实际：%v", i, got)
		}
	}
	if got := m.feed("新的密码："); len(got) != 0 {
		t.Fatalf("喂满 max 之后不该再喂，实际：%v", got)
	}
}

// 当前的密码最多只喂一次：它不会重问，多喂只会把密码写进后面的读取。
func TestPasswdMatcherCurrentFeedsOnce(t *testing.T) {
	m := newPasswdMatcher(testPrompts(), testEnterKeywords, MatchOptions{})
	m.feed("密码已过期")
	if got := m.feed("当前的密码："); len(got) != 1 {
		t.Fatalf("第一次应喂当前密码，实际：%v", got)
	}
	if got := m.feed("当前的密码："); len(got) != 0 {
		t.Fatalf("max=1 时第二次不该再喂，实际：%v", got)
	}
}

// 一条提示配多个关键词，任一命中即可。
func TestPasswdMatcherMultipleKeywords(t *testing.T) {
	m := newPasswdMatcher(testPrompts(), testEnterKeywords, MatchOptions{})
	m.feed("密码已过期")
	if got := m.feed("Current password: "); len(got) != 1 || got[0] != PasswdValueCurrent {
		t.Fatalf("英文提示也该认出来，实际：%v", got)
	}
}

// 一次推来的块里连着两条提示：两条都要喂，且按出现先后。
func TestPasswdMatcherTwoPromptsOneChunk(t *testing.T) {
	m := newPasswdMatcher(testPrompts(), testEnterKeywords, MatchOptions{})
	got := m.feed("密码已过期：当前的密码：新的密码：")
	if len(got) != 2 || got[0] != PasswdValueCurrent || got[1] != PasswdValueNew {
		t.Fatalf("一块里两条提示应按序各喂一次，实际：%v", got)
	}
}

// 全表喂满之后不再匹配（尾部与游标都不再需要）。
func TestPasswdMatcherExhausted(t *testing.T) {
	prompts := &PasswdPrompts{Steps: []PasswdStep{
		{Keywords: []string{"新的密码"}, Value: PasswdValueNew, Max: 1},
	}}
	m := newPasswdMatcher(prompts, testEnterKeywords, MatchOptions{})
	m.feed("密码已过期")
	if got := m.feed("新的密码："); len(got) != 1 {
		t.Fatalf("应喂一次，实际：%v", got)
	}
	if !m.exhausted() {
		t.Fatal("全表用满后应判成 exhausted")
	}
	if got := m.feed("新的密码："); len(got) != 0 {
		t.Fatalf("用满之后不该再喂，实际：%v", got)
	}
}

// 匹配口径两个开关都管改密的提示：大小写不敏感是默认，开启正则就按正则。
func TestPasswdMatcherMatchOptions(t *testing.T) {
	// 默认：子串包含，不区分大小写
	loose := newPasswdMatcher(testPrompts(), testEnterKeywords, MatchOptions{})
	loose.feed("密码已过期")
	if got := loose.feed("NEW PASSWORD:"); len(got) != 1 {
		t.Fatalf("默认不区分大小写：应命中新密码那一条，实际：%v", got)
	}

	// 正则：关键词当正则用，"新.密码" 能匹配「新的密码」
	regexPrompts := &PasswdPrompts{Steps: []PasswdStep{
		{Keywords: []string{"新.密码"}, Value: PasswdValueNew, Max: 1},
	}}
	regexMatcher := newPasswdMatcher(regexPrompts, testEnterKeywords, MatchOptions{Regex: true})
	regexMatcher.feed("密码已过期")
	if got := regexMatcher.feed("新的密码："); len(got) != 1 {
		t.Fatalf("按正则匹配：应命中新密码那一条，实际：%v", got)
	}

	// 区分大小写：大小写对不上就不该命中
	sensitive := newPasswdMatcher(testPrompts(), testEnterKeywords, MatchOptions{CaseSensitive: true})
	sensitive.feed("密码已过期")
	if got := sensitive.feed("NEW PASSWORD:"); len(got) != 0 {
		t.Fatalf("区分大小写时不该命中，实际：%v", got)
	}
}

// 提示词表加载：字段缺一即报错，报错要指名第几条。
func TestLoadPasswdPrompts(t *testing.T) {
	write := func(t *testing.T, body string) string {
		t.Helper()
		path := t.TempDir() + "/passwd_prompts.conf"
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		return path
	}

	ok := write(t, `[[step]]
keywords = ['新的密码']
value = 'new'
max = 2
`)
	prompts, err := LoadPasswdPrompts(ok)
	if err != nil {
		t.Fatalf("应加载成功：%v", err)
	}
	if len(prompts.Steps) != 1 || prompts.Steps[0].Max != 2 || prompts.Steps[0].Value != PasswdValueNew {
		t.Fatalf("解析结果不对：%+v", prompts.Steps)
	}
	if prompts.stepBudget() != 2 {
		t.Fatalf("喂入预算应为 2，实际 %d", prompts.stepBudget())
	}

	bad := map[string]string{
		"一条 step 都没有":   ``,
		"缺 keywords":    "[[step]]\nvalue = 'new'\nmax = 1\n",
		"value 写错":      "[[step]]\nkeywords = ['x']\nvalue = 'newest'\nmax = 1\n",
		"max 为 0":       "[[step]]\nkeywords = ['x']\nvalue = 'new'\nmax = 0\n",
		"keywords 里有空项": "[[step]]\nkeywords = ['x', ' ']\nvalue = 'new'\nmax = 1\n",
		"多写了个不认识的字段":    "[[step]]\nkeywords = ['x']\nvalue = 'new'\nmax = 1\nwat = 1\n",
	}
	for name, body := range bad {
		if _, err := LoadPasswdPrompts(write(t, body)); err == nil {
			t.Fatalf("%s 时应报错", name)
		}
	}
}

// 报错文案要指得出是哪一条。
func TestLoadPasswdPromptsNamesTheStep(t *testing.T) {
	path := t.TempDir() + "/passwd_prompts.conf"
	body := "[[step]]\nkeywords = ['x']\nvalue = 'new'\nmax = 1\n\n[[step]]\nkeywords = ['y']\nvalue = 'oops'\nmax = 1\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := LoadPasswdPrompts(path)
	if err == nil || !strings.Contains(err.Error(), "第 2 条") {
		t.Fatalf("应报出第 2 条，实际：%v", err)
	}
}
