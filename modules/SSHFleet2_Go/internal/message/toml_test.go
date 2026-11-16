package message

import "testing"

// 词法器那五句与配置那一屏的头一句（自定义区）：期望值照迁移前的 `fmt.Errorf` 逐字抄，
// 不拿被测函数再算一遍——这批的判据就是「字节不变」。
func TestTOMLErrorText(t *testing.T) {
	cases := []struct {
		name string
		got  string
		want string
	}{
		{"节名是空的", TOMLSectionNameEmpty(3), "第 3 行的节名是空的"},
		{"既不是节也不是键值对", TOMLNotSectionNorKeyValue(7, "孤零零一行"), "第 7 行既不是节也不是键值对：孤零零一行"},
		{"键名是空的", TOMLKeyNameEmpty(12), "第 12 行的键名是空的"},
		{"键在节之前", TOMLKeyBeforeAnySection(1, "port"), "第 1 行的键 port 出现在任何节之前"},
		{"普通节里写两次", TOMLDuplicateKey(4, "port", "account", 0), "第 4 行的键 port 在 [account] 里写了两次"},
		{"数组表某条里写两次", TOMLDuplicateKey(9, "name", "category", 2), "第 9 行的键 name 在 [[category]] 第 2 条 里写了两次"},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Fatalf("%s：\n got  %q\n want %q", c.name, c.got, c.want)
		}
	}

	if got := TOMLParseFailed(TOMLKeyNameEmpty(12)); got !=
		"解析 TOML 失败：第 12 行的键名是空的\n"+
			"提示：多数是引号或方括号没配对，照随包的配置文件核一遍" {
		t.Fatalf("配置那一屏：%q", got)
	}
}
