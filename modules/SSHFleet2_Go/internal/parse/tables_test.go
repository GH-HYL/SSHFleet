package parse

import "testing"

// 判据表的编译口径（自定义区）：老版编译危险规则时统一加 `(?i)`（对位旧实现的
// re.IGNORECASE）。不照搬就会漏掉 `RM -RF /etc` 这一类大写写法——那是同一个命令。
func TestBuildRuleCompilesCaseInsensitive(t *testing.T) {
	rule, err := buildRule(map[string]string{
		"name":       "递归删除系统目录",
		"regex":      `\brm\s+-rf\s+/etc(?:/\*)?(?:[\s;&|)]|$)`,
		"risk_level": "high",
	})
	if err != nil {
		t.Fatalf("这条规则本身是合法的，不该报错：%v", err)
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
