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
		{"10.0.0.1,22,root,hunter2", "10.0.0.1,22,root,hu****r2"},
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

// 命令行脱敏：三个位置会带秘密，其余段一个字都不许动。
//
// 期望值写成字面量：遮只有一种形态（前二后二），内联记录那两段也一样（建站 07 第六节）。
func TestCommandLineMasksCredentialPositions(t *testing.T) {
	cases := []struct {
		name string
		argv []string
		want []string
	}{
		{
			name: "内联清单",
			argv: []string{"sshfleet2", "-f", "10.0.0.1,22,root,hunter2", "-c", "uptime"},
			want: []string{"sshfleet2", "-f", "10.0.0.1,22,root,hu****r2", "-c", "uptime"},
		},
		{
			name: "等号写法的内联清单",
			argv: []string{"sshfleet2", "-f=10.0.0.1,22,root,hunter2", "-c", "uptime"},
			want: []string{"sshfleet2", "-f=10.0.0.1,22,root,hu****r2", "-c", "uptime"},
		},
		{
			name: "改密的新密码（空格写法）",
			argv: []string{"sshfleet2", "--change-password", "Ne#wpass", "-f", "u1.csv"},
			want: []string{"sshfleet2", "--change-password", "Ne****ss", "-f", "u1.csv"},
		},
		{
			name: "改密的新密码（等号写法）",
			argv: []string{"sshfleet2", "--change-password=Ne#wpass"},
			want: []string{"sshfleet2", "--change-password=Ne****ss"},
		},
		{
			name: "命令正文里长得像内联清单的那一段不动",
			argv: []string{"sshfleet2", "-c", "echo 10.0.0.1,22,root,hunter2"},
			want: []string{"sshfleet2", "-c", "echo 10.0.0.1,22,root,hunter2"},
		},
	}
	for _, c := range cases {
		got := CommandLine(c.argv)
		if len(got) != len(c.want) {
			t.Fatalf("%s：段数不对：%v", c.name, got)
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Fatalf("%s：第 %d 段\n got=%q\nwant=%q", c.name, i, got[i], c.want[i])
			}
		}
	}
}

// 遮完不该动调用方的切片：报告与日志各拿一份，谁改都不该影响对方。
func TestCommandLineDoesNotTouchCallerSlice(t *testing.T) {
	argv := []string{"sshfleet2", "-f", "10.0.0.1,22,root,hunter2"}
	_ = CommandLine(argv)
	if argv[2] != "10.0.0.1,22,root,hunter2" {
		t.Fatalf("原切片被改了：%v", argv)
	}
}

// 从采集文本里擦掉明文，擦出来与遮一个值同一个形态——整个工程只有一种遮法。
func TestScrubUsesSameForm(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"长密码留前二后二", "远端回显：pw=hunter2 完", "远端回显：pw=hu****r2 完"},
		{"短密码整段抹", "pw: abc", "pw: ****"},
		{"多处出现都擦掉", "hunter2 与 hunter2", "hu****r2 与 hu****r2"},
		{"空秘密不参与", "原样", "原样"},
	}
	for _, c := range cases {
		secrets := []string{"hunter2", ""}
		if c.name == "短密码整段抹" {
			secrets = []string{"abc"}
		}
		if got := Scrub(c.in, secrets...); got != c.want {
			t.Fatalf("%s：got=%q want=%q", c.name, got, c.want)
		}
	}
}

// 引号不成对的值不遮、原样返回：读不出就不是内联清单。
//
// 这四条与 `criterion.ReadCSVRows` 那条用例是同一组输入——取值侧读不出、遮罩侧也不认，
// 两边同一把尺子（本议题 `audit.md` §三·1 里的那四条）。
func TestInlineRecordIfLeavesMalformedAlone(t *testing.T) {
	for _, raw := range []string{
		`10.0.0.1,22,root,p"w`,
		`10.0.0.1,22,root,"p"w`,
		`10.0.0.1,22,root,pw"`,
		`10.0.0.1,22,ro"ot,pw`,
	} {
		if got := InlineRecordIf(raw); got != raw {
			t.Errorf("畸形值不该被遮：%q → %q", raw, got)
		}
	}
}
