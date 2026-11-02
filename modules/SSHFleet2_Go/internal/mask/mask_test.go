package mask

import (
	"strings"
	"testing"
)

// 遮秘密那一件事：遮过之后找不到原值，没遮的部分一字不变。
func TestSecretKeepsShapeNotContent(t *testing.T) {
	cases := []struct{ in, want string }{
		{"pw", "****"},                // 四个字符以内整个遮掉
		{"1234", "****"},              //
		{"Sshfleet#2026", "Ss****26"}, // 再长的留前二后二，够对上号、不足以还原
	}
	for _, c := range cases {
		if got := Secret(c.in); got != c.want {
			t.Fatalf("Secret(%q) = %q，want %q", c.in, got, c.want)
		}
		if strings.Contains(Secret(c.in), "") && len([]rune(c.in)) > 4 {
			if strings.HasSuffix(c.want, c.in) {
				t.Fatalf("原值整段留在里面了：%q", Secret(c.in))
			}
		}
	}
}

// 内联清单：只遮凭据那两段，其余原样；读不成一条记录的不动。
func TestInlineRecordMasksCredentialColumns(t *testing.T) {
	cases := []struct{ in, want string }{
		{"10.0.0.1,22,root,pw", "10.0.0.1,22,root,****"},
		{"10.0.0.1,22,root,pw,/k/id_rsa,pass", "10.0.0.1,22,root,****,/k/id_rsa,****"},
		{"10.0.0.1", "10.0.0.1"},                                            // 没有凭据段，不遮
		{"这不是一条记录", "这不是一条记录"},                                              // 读不成，原样
		{"10.0.0.1,22,root,\"p,w\",/k,kp", "10.0.0.1,22,root,****,/k,****"}, // 引号里的逗号不算分列
	}
	for _, c := range cases {
		if got := InlineRecordIf(c.in); got != c.want {
			t.Fatalf("InlineRecordIf(%q)\n got=%q\nwant=%q", c.in, got, c.want)
		}
	}
}

// 遮过之后原文不许出现在结果里——这条是这一整件事的目的。
func TestInlineRecordHidesPassword(t *testing.T) {
	const in = "10.0.0.1,22,root,hunter2,/k/id_rsa,secretpass"
	got := InlineRecord(in)
	for _, secret := range []string{"hunter2", "secretpass"} {
		if strings.Contains(got, secret) {
			t.Fatalf("%q 还在结果里：%s", secret, got)
		}
	}
	if !strings.Contains(got, "10.0.0.1") || !strings.Contains(got, "root") {
		t.Fatalf("认机器的几段被抹掉了：%s", got)
	}
}
