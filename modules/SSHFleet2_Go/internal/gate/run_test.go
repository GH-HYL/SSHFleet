package gate

import (
	"regexp"
	"strings"
	"testing"

	"sshfleet2/internal/contract"
	"sshfleet2/internal/message"
)

// 参数搭配那一段（自定义区）：递一张纸，收一个拒绝。
//
// 期望值只钉每一屏的**第一行**（问题那句）——它才是「哪一条没过」的判据；后面的「原因／提示」
// 由 message 拼，那边自有逐字断言。这样这里红了就说明闸门判错了条，不是话写错了。
//
// 纸是手搓的（临时用具，真纸随阶段 04），只填本案要看的那几格。

func planOf(mode contract.Mode, edit func(*contract.Plan)) *contract.Plan {
	// Yes 为真：这些用例判的是「拦不拦」，不该真去等一行输入（确认屏由 07 号单的用例管）。
	p := &contract.Plan{Mode: mode, Run: contract.RunParams{Yes: true}}
	if edit != nil {
		edit(p)
	}
	return p
}

// ansiEscape 去掉上色码：闸门造的话一律带色，判据只看字。
var ansiEscape = regexp.MustCompile("\x1b\\[[0-9;]*m")

// firstLine 取一屏话的问题句：老版那副外壳之后、第一个换行为止。
func firstLine(t *testing.T, say message.Line) string {
	t.Helper()
	line := ansiEscape.ReplaceAllString(say.Text(), "")
	if i := strings.Index(line, "\n"); i >= 0 {
		line = line[:i]
	}
	// 外壳是 `[ERROR] [function:cli] `，问题句在它后面。
	const shell = "[function:cli] "
	if i := strings.Index(line, shell); i >= 0 {
		line = line[i+len(shell):]
	}
	return line
}

func TestRefuseArguments(t *testing.T) {
	cases := []struct {
		name string
		plan *contract.Plan
		want string
	}{
		{
			name: "模式零个",
			plan: planOf(contract.ModeNone, nil),
			want: "选项/参数合规性检查未通过：执行模式选项：-c、-s、-u、-d、--change-password 互斥，只能指定一个",
		},
		{
			name: "模式多个",
			plan: planOf(contract.ModeNone, func(p *contract.Plan) {
				p.Problems = append(p.Problems, contract.Problem{Kind: contract.ProblemMultiMode})
			}),
			want: "选项/参数合规性检查未通过：执行模式选项：-c、-s、-u、-d、--change-password 互斥，只能指定一个",
		},
		{
			name: "认不出的选项",
			plan: planOf(contract.ModeCommand, func(p *contract.Plan) {
				p.Task.Command = &contract.CommandTask{Text: "ls"}
				p.Problems = append(p.Problems, contract.Problem{Kind: contract.ProblemUnknownOption, Option: "--zzz"})
			}),
			want: "选项/参数解析失败：不认识的选项：--zzz（使用 -h 查看帮助）",
		},
		{
			name: "--no-shell 配 -u",
			plan: planOf(contract.ModeUpload, func(p *contract.Plan) {
				p.Run.NoShell = true
				p.Task.Upload = &contract.UploadTask{Source: "."}
				p.Task.Path = "/tmp/"
			}),
			want: "选项/参数合规性检查未通过：--no-shell 只能和 -c 一起使用",
		},
		{
			name: "改密配 -p",
			plan: planOf(contract.ModePasswd, func(p *contract.Plan) {
				p.Task.Passwd = &contract.PasswdTask{Raw: "pw"}
				p.Task.Path = "/tmp"
			}),
			want: "选项/参数合规性检查未通过：--change-password 不能和 -p 一起用",
		},
		{
			name: "上传目录不是绝对路径",
			plan: planOf(contract.ModeUpload, func(p *contract.Plan) {
				p.Task.Upload = &contract.UploadTask{Source: "."}
				p.Task.Path = "rel"
			}),
			want: "选项/参数合规性检查未通过：-p 参数指定的上传目录必须是绝对路径，当前值：rel",
		},
		{
			name: "上传目录不以斜杠收尾",
			plan: planOf(contract.ModeUpload, func(p *contract.Plan) {
				p.Task.Upload = &contract.UploadTask{Source: "."}
				p.Task.Path = "/tmp"
			}),
			want: "选项/参数合规性检查未通过：上传模式：-p 必须是以 / 结尾的目录，当前值：/tmp",
		},
		{
			name: "-p 配 -c",
			plan: planOf(contract.ModeCommand, func(p *contract.Plan) {
				p.Task.Command = &contract.CommandTask{Text: "ls"}
				p.Task.Path = "/tmp"
			}),
			want: "选项/参数合规性检查未通过：-p 参数不能搭配 -c 或 -s 使用",
		},
		{
			name: "-d 缺 -p",
			plan: planOf(contract.ModeDownload, func(p *contract.Plan) {
				p.Task.Download = &contract.DownloadTask{RemotePath: "/etc/passwd"}
			}),
			want: "选项/参数合规性检查未通过：-d 参数必须搭配 -p 参数使用",
		},
		{
			name: "-u 缺 -p",
			plan: planOf(contract.ModeUpload, func(p *contract.Plan) {
				p.Task.Upload = &contract.UploadTask{Source: "."}
			}),
			want: "选项/参数合规性检查未通过：-u 参数必须搭配 -p 参数使用",
		},
		{
			name: "两个身份开关同给",
			plan: planOf(contract.ModeCommand, func(p *contract.Plan) {
				p.Task.Command = &contract.CommandTask{Text: "ls"}
				p.Problems = append(p.Problems, contract.Problem{
					Kind:   contract.ProblemFlagConflict,
					Option: "--sudo --no-sudo",
				})
			}),
			want: "选项/参数合规性检查未通过：--sudo 与 --no-sudo 只能给一个（两个都不给=按配置里的值）",
		},
		{
			name: "多余的位置参数",
			plan: planOf(contract.ModeCommand, func(p *contract.Plan) {
				p.Task.Command = &contract.CommandTask{Text: "echo hi"}
				p.Problems = append(p.Problems, contract.Problem{
					Kind:   contract.ProblemExtraArgs,
					Option: "-c",
					Raw:    "extra",
				})
			}),
			want: "选项/参数解析失败：-c 的值后面还有一段没法识别：extra",
		},
		{
			name: "-c 只有空白",
			plan: planOf(contract.ModeCommand, func(p *contract.Plan) {
				p.Task.Command = &contract.CommandTask{Text: "   "}
			}),
			want: "选项/参数合规性检查未通过：-c 参数不能为空，请提供要执行的命令",
		},
		{
			name: "数值选项认不出",
			plan: planOf(contract.ModeCommand, func(p *contract.Plan) {
				p.Task.Command = &contract.CommandTask{Text: "ls"}
				p.Problems = append(p.Problems, contract.Problem{
					Kind: contract.ProblemBadValue, Option: "-n", Raw: "abc", Subject: contract.SubjectConcurrency,
				})
			}),
			want: "选项/参数合规性检查未通过：-n 参数格式错误，并发连接数必须是正整数，当前值：abc",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r, note := Run(c.plan)
			if r == nil {
				t.Fatalf("应当拒绝，实际放行")
			}
			if note.Text() != "" {
				t.Fatalf("拒绝时不该带回日志留痕：%q", note.Text())
			}
			if !r.Say().Stderr() {
				t.Fatalf("参数不合规要写标准错误，实际写的是标准输出")
			}
			if got := firstLine(t, r.Say()); got != c.want {
				t.Fatalf("问题句不对：\n got  %q\n want %q", got, c.want)
			}
		})
	}
}

// 该放行的要放行：这条挡着「一律拒绝」那种退化解法。
func TestPassesCleanPlan(t *testing.T) {
	plan := planOf(contract.ModeCommand, func(p *contract.Plan) {
		p.Task.Command = &contract.CommandTask{Text: "echo hi"}
	})
	if r, _ := Run(plan); r != nil {
		t.Fatalf("干净的命令不该拒：%q", r.Say().Text())
	}

	plan = planOf(contract.ModeUpload, func(p *contract.Plan) {
		p.Task.Upload = &contract.UploadTask{Source: "."}
		p.Task.Path = "/tmp/"
	})
	if r, _ := Run(plan); r != nil {
		t.Fatalf("干净的上传不该拒：%q", r.Say().Text())
	}

	plan = planOf(contract.ModeDownload, func(p *contract.Plan) {
		p.Task.Download = &contract.DownloadTask{RemotePath: "/etc/passwd"}
		p.Task.Path = "/tmp"
	})
	if r, _ := Run(plan); r != nil {
		t.Fatalf("干净的下载不该拒：%q", r.Say().Text())
	}
}
