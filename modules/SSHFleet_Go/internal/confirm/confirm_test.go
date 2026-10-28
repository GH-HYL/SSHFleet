package confirm

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sshfleet/internal/cli"
	"sshfleet/internal/common"
	"sshfleet/internal/nodelist"
	"sshfleet/internal/ssh"
)

// 确认屏渲染：屏上每一行都必须让人对得上自己敲的选项。
//
// 本文件测的是**渲染出来的文本**（paramScreenText），不是内部结构——
// 接缝与 cli.UsageText 同形：渲染成字符串，打印交给调用方。

func testNodes() *nodelist.Nodes {
	return &nodelist.Nodes{Items: []nodelist.NodeInfo{{IP: "172.28.118.49"}}}
}

// 命令模式的取值行标签是「命令内容」——点出 -c 收的是**文本**。
//
// 此前标签写作「执行命令」，太中性：使用者把文件名误敲给 -c 时，
// 屏上「执行命令-→ afile.txt」既读得通「命令内容是 afile.txt」，
// 也读得通「去执行 afile.txt」，扫一眼就过去了（2026-10-15 审计实测）。
// 改成「命令内容」后与「脚本路径」「本地路径」同构：都是「这格是什么」。
func TestParamScreenCommandRowLabel(t *testing.T) {
	a := &cli.Args{Command: "uptime"}
	text := paramScreenText(a, testNodes(), "", "")

	if !strings.Contains(text, "命令内容") {
		t.Fatalf("命令模式的取值行应标为「命令内容」，实际渲染：\n%s", text)
	}
	if strings.Contains(text, "执行命令") {
		t.Fatalf("不该再出现「执行命令」这个标签，实际渲染：\n%s", text)
	}
}

// 五种模式的取值行标签都要是「这格是什么」的短名词，且互不相同——
// 使用者靠它认自己敲的是哪一个选项。
func TestParamScreenRowLabelsPerMode(t *testing.T) {
	cases := []struct {
		name  string
		args  *cli.Args
		wants []string
	}{
		{"命令", &cli.Args{Command: "uptime"}, []string{"命令内容"}},
		{"脚本", &cli.Args{Script: "good.sh"}, []string{"脚本路径"}},
		{"上传", &cli.Args{Upload: "afile.txt", Path: "/opt/"}, []string{"本地路径", "远程路径"}},
		{"下载", &cli.Args{Download: "/var/log", Path: "./logs/"}, []string{"远程路径", "本地路径"}},
		{"改密", &cli.Args{ChangePasswordRaw: "N3wpw#2026"}, []string{"新密码"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			text := paramScreenText(c.args, testNodes(), "", "")
			for _, w := range c.wants {
				if !strings.Contains(text, w) {
					t.Fatalf("%s 模式的取值行应含 %q，实际渲染：\n%s", c.name, w, text)
				}
			}
		})
	}
}

// 上传内容清单的标签按规范口径称「选项」——`-x` 这类开关一律称选项，
// 只有选项后面跟的值才叫参数（CHANGELOG 9.1.0 定）。
//
// 断言的是**渲染出来的文本**，不是 uploadTreeTitle 常量本身：断言常量等于拿被测代码
// 验被测代码，改坏了它跟着坏、永远绿（规范「期望值 MUST 独立于被测代码」）。
func TestUploadTreeTitleUsesOption(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "afile.txt"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}

	text, err := uploadTreeText(dir)
	if err != nil {
		t.Fatalf("渲染上传清单不该报错：%v", err)
	}
	if !strings.Contains(text, "-u 选项") {
		t.Fatalf("上传内容清单的标签应称「-u 选项」，实际渲染：\n%s", text)
	}
	if strings.Contains(text, "-u 参数") {
		t.Fatalf("不该再出现「-u 参数」，实际渲染：\n%s", text)
	}
	// 顺带确认这棵树真的把它该显示的东西显示出来了
	if !strings.Contains(text, "afile.txt") {
		t.Fatalf("清单里应列出文件名，实际渲染：\n%s", text)
	}
}

// 改密模式的「新密码」那行显示的是**这次要设的新密码**的脱敏形态，与加密开关无关。
//
// 加密模式下 --change-password 收的是密文文件路径，真正的密码要经 credential 解出来
// 才作数（cli.Args.NewPassword）。屏上若取旗标原值，加密时显示的就是一串路径的脱敏——
// 既不是密码、也对不上任何东西，确认屏上根本无从核对（2026-10-21 实测）。
//
// 期望值按 MaskSecret 的公开规则手算：前 2 位 + **** + 后 2 位。
func TestParamScreenPasswdRowShowsRealNewPassword(t *testing.T) {
	cases := []struct {
		name string
		args *cli.Args
		want string // 屏上「新密码」那一格的值
	}{
		{
			name: "加密模式：旗标值是密文文件路径，屏上显示解出来的新密码",
			args: &cli.Args{
				ChangePasswordRaw: "/home/hyl/secret/new_pw.txt",
				NewPassword:       "NewPass#2026",
			},
			want: "Ne****26",
		},
		{
			name: "明文模式：旗标值就是新密码",
			args: &cli.Args{
				ChangePasswordRaw: "NewPass#2026",
				NewPassword:       "NewPass#2026",
			},
			want: "Ne****26",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			text := paramScreenText(c.args, testNodes(), "", "")
			if !strings.Contains(text, c.want) {
				t.Fatalf("「新密码」应显示 %s，实际渲染：\n%s", c.want, text)
			}
			// 路径的脱敏形态（/h****xt）不该出现在屏上：屏上那格是密码，不是文件
			if strings.Contains(text, "/h****xt") {
				t.Fatalf("屏上不该出现密文文件路径的脱敏形态，实际渲染：\n%s", text)
			}
		})
	}
}

// 参数屏的「执行身份」行：-c / -s 不再显示（生效时多半与登录用户一致，--no-shell 下
// 又整个不生效——不生效的身份摆一行没有信息量，⚠ 提醒与日志里都有）；-u / -d / 改密保留。
func TestParamScreenIdentityRowPerMode(t *testing.T) {
	hide := []struct {
		name string
		args *cli.Args
	}{
		{"命令", &cli.Args{Command: "uptime"}},
		{"脚本", &cli.Args{Script: "good.sh"}},
	}
	for _, c := range hide {
		t.Run(c.name, func(t *testing.T) {
			if text := paramScreenText(c.args, testNodes(), "", ""); strings.Contains(text, "执行身份") {
				t.Fatalf("%s 模式不该再显示「执行身份」行，实际渲染：\n%s", c.name, text)
			}
		})
	}
	keep := []struct {
		name string
		args *cli.Args
	}{
		{"上传", &cli.Args{Upload: "afile.txt", Path: "/opt/", Sudo: true}},
		{"下载", &cli.Args{Download: "/var/log", Path: "./logs/", Sudo: true}},
		{"改密", &cli.Args{ChangePasswordRaw: "N3wpw#2026", Sudo: true}},
	}
	for _, c := range keep {
		t.Run(c.name, func(t *testing.T) {
			if text := paramScreenText(c.args, testNodes(), "", ""); !strings.Contains(text, "执行身份") {
				t.Fatalf("%s 模式应保留「执行身份」行，实际渲染：\n%s", c.name, text)
			}
		})
	}
}

// english = true 时代填值（第 1 列）含中文的拦截门：默认回车（n）取消执行；
// 触发词里的中文不拦（服务端报文不受 english 影响）；english = false 不进这段检查。
func TestGateChineseAnswerValues(t *testing.T) {
	a := &cli.Args{English: true, Answers: []ssh.Answer{
		{Value: "yes", Triggers: []string{"password:"}},
		{Value: "是", Triggers: []string{"选择架构"}},
	}}
	in := &common.Interactor{In: strings.NewReader("\n"), Out: &strings.Builder{}}
	err := gateChineseAnswerValues(a, in, nil)
	if !errors.Is(err, common.ErrCancelled) {
		t.Fatalf("默认回车（n）应取消执行，实际 err=%v", err)
	}
	out := in.Out.(*strings.Builder).String()
	for _, want := range []string{"english = true", "第 2 条代填值是「是」", "触发词不受此限"} {
		if !strings.Contains(out, want) {
			t.Fatalf("提醒里应交代 %q，实际输出：\n%s", want, out)
		}
	}

	// 触发词是中文、代填值是英文：不拦，答 y 继续
	a = &cli.Args{English: true, Answers: []ssh.Answer{{Value: "yes", Triggers: []string{"选择架构"}}}}
	in = &common.Interactor{In: strings.NewReader("y\n"), Out: &strings.Builder{}}
	if err := gateChineseAnswerValues(a, in, nil); err != nil {
		t.Fatalf("触发词里的中文不该拦，实际 err=%v", err)
	}

	// english = false：根本不进检查，即便代填值是中文
	a = &cli.Args{English: false, Answers: []ssh.Answer{{Value: "是", Triggers: []string{"选择架构"}}}}
	in = &common.Interactor{In: strings.NewReader("\n"), Out: &strings.Builder{}}
	if err := gateChineseAnswerValues(a, in, nil); err != nil {
		t.Fatalf("english = false 时不该进检查，实际 err=%v", err)
	}
	out = in.Out.(*strings.Builder).String()
	if strings.Contains(out, "english") {
		t.Fatalf("english = false 时不应有任何提醒输出，实际：\n%s", out)
	}
}
