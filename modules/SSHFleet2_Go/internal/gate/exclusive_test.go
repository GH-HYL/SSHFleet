package gate

// 建站 05 的五条互斥与 `-a` 的两条判据（08 号单）。
//
// 与 run_test.go 同一条口径：只钉每一屏的**问题句**，后面那些「原因／提示」由 message 拼，
// 那边自有逐字断言。这里红了说明闸门判错了条，不是话写错了。

import (
	"strings"
	"testing"

	"sshfleet2/internal/contract"
)

// 五条互斥：每一条都是「这次运行该长什么样」自相矛盾，都得在确认屏之前拦下。
func TestRefuseExclusivePairs(t *testing.T) {
	cases := []struct {
		name string
		plan *contract.Plan
		want string
	}{
		{
			name: "-a 与上传",
			plan: planOf(contract.ModeUpload, func(p *contract.Plan) {
				p.Task.Upload = &contract.UploadTask{Source: "."}
				p.Task.Path = "/tmp/"
				p.Run.Answer = "y,继续"
			}),
			want: "选项/参数合规性检查未通过：-a 不能和 -u 一起用",
		},
		{
			name: "-a 与下载",
			plan: planOf(contract.ModeDownload, func(p *contract.Plan) {
				p.Task.Download = &contract.DownloadTask{RemotePath: "/etc/passwd"}
				p.Task.Path = "/tmp"
				p.Run.Answer = "y,继续"
			}),
			want: "选项/参数合规性检查未通过：-a 不能和 -d 一起用",
		},
		{
			name: "-a 与 --no-shell",
			plan: planOf(contract.ModeCommand, func(p *contract.Plan) {
				p.Task.Command = &contract.CommandTask{Text: "install.sh"}
				p.Run.NoShell = true
				p.Run.Answer = "y,继续"
			}),
			want: "选项/参数合规性检查未通过：--no-shell 不能和 -a 一起使用",
		},
		{
			name: "改密与身份开关",
			plan: planOf(contract.ModePasswd, func(p *contract.Plan) {
				p.Task.Passwd = &contract.PasswdTask{Raw: "pw"}
				p.Run.SudoGiven = true
			}),
			want: "选项/参数合规性检查未通过：--change-password 不能和 --sudo / --no-sudo 一起用",
		},
		{
			name: "改密与 --no-shell",
			plan: planOf(contract.ModePasswd, func(p *contract.Plan) {
				p.Task.Passwd = &contract.PasswdTask{Raw: "pw"}
				p.Run.NoShell = true
			}),
			want: "选项/参数合规性检查未通过：--no-shell 只能和 -c 一起使用",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r, _ := Run(c.plan, false)
			if r == nil {
				t.Fatal("应当拒绝，实际放行")
			}
			if got := firstLine(t, r.Say()); got != c.want {
				t.Fatalf("问题句不对：\n got  %q\n want %q", got, c.want)
			}
		})
	}
}

// 身份开关没在命令行上给过时不报：配置默认是 root 也一样。
func TestPasswdWithConfigRootPasses(t *testing.T) {
	plan := planOf(contract.ModePasswd, func(p *contract.Plan) {
		p.Task.Passwd = &contract.PasswdTask{Raw: "pw"}
		p.Run.Sudo = true // 配置里的值，命令行没给
	})
	if r, _ := Run(plan, false); r != nil {
		t.Fatalf("配置默认是 root 不该算「给了身份开关」：%q", r.Say().Text())
	}
}

// `-a` 的下发行超过上限：归拒绝，量的是整条串。
func TestRefuseAnswerTooLong(t *testing.T) {
	plan := planOf(contract.ModeCommand, func(p *contract.Plan) {
		p.Task.Command = &contract.CommandTask{Text: "install.sh"}
		p.Run.Answer = "y,继续"
		p.Run.Downlink = strings.Repeat("x", answerLimit+1)
	})
	r, _ := Run(plan, false)
	if r == nil {
		t.Fatal("超长应当拒绝")
	}
	body := ansiEscape.ReplaceAllString(r.Say().Text(), "")
	for _, want := range []string{"-a 参数下发的命令太长", "上限 98304 字节"} {
		if !strings.Contains(body, want) {
			t.Fatalf("话里少了 %q：%s", want, body)
		}
	}
}

// 刚好等于上限要放行：判据是「超过」。
func TestAnswerAtLimitPasses(t *testing.T) {
	plan := planOf(contract.ModeCommand, func(p *contract.Plan) {
		p.Task.Command = &contract.CommandTask{Text: "install.sh"}
		p.Run.Answer = "y,继续"
		p.Run.Downlink = strings.Repeat("x", answerLimit)
	})
	if r, _ := Run(plan, false); r != nil {
		t.Fatalf("等于上限不该拒：%q", r.Say().Text())
	}
}

// 表外解释器：解析站只记事实，闸门照它拒，并列出目前支持的那几个。
func TestRefuseAnswerUnsupportedInterpreter(t *testing.T) {
	plan := planOf(contract.ModeCommand, func(p *contract.Plan) {
		p.Task.Command = &contract.CommandTask{Text: "install.sh"}
		p.Run.Answer = "y,继续"
		p.Problems = append(p.Problems, contract.Problem{
			Kind:   contract.ProblemInterpreterUnsupported,
			Option: "--answer",
			Raw:    "tclsh",
		})
	})
	r, _ := Run(plan, false)
	if r == nil {
		t.Fatal("表外解释器应当拒绝")
	}
	body := ansiEscape.ReplaceAllString(r.Say().Text(), "")
	for _, want := range []string{"-a 代填不支持解释器", "tclsh", "目前支持：bash", "php"} {
		if !strings.Contains(body, want) {
			t.Fatalf("话里少了 %q：%s", want, body)
		}
	}
}

// —— 09 号单：三道门与并发建议（走 --yes 那条路，不问人、只留痕） ——

// 上传并发建议：建议值不等于当前生效值时，非交互自动采纳并留痕；纸上那一格跟着改。
func TestUploadAdviceAdoptedUnderYes(t *testing.T) {
	plan := planOf(contract.ModeUpload, func(p *contract.Plan) {
		p.Task.Upload = &contract.UploadTask{Source: "./dist", TotalBytes: 8192}
		p.Task.Path = "/opt/app/"
		p.Nodes = []contract.Node{{IP: "a"}, {IP: "b"}, {IP: "c"}, {IP: "d"}, {IP: "e"}}
		p.Run.ConcurrencyAdvice = 4
	})
	r, note := Run(plan, false)
	if r != nil {
		t.Fatalf("有建议也不该拒：%q", r.Say().Text())
	}
	if plan.Run.Concurrency != 4 {
		t.Fatalf("非交互应当自动采纳建议，实际并发 %d", plan.Run.Concurrency)
	}
	if !strings.Contains(note.Text(), "自动采纳上传并发建议：4 个") {
		t.Fatalf("留痕不对：%q", note.Text())
	}
}

// 建议值等于当前生效值时闭嘴：同一个数没什么可建议的。
func TestUploadAdviceSilentWhenEqual(t *testing.T) {
	plan := planOf(contract.ModeUpload, func(p *contract.Plan) {
		p.Task.Upload = &contract.UploadTask{Source: "./dist", TotalBytes: 8192}
		p.Task.Path = "/opt/app/"
		p.Nodes = []contract.Node{{IP: "a"}, {IP: "b"}, {IP: "c"}, {IP: "d"}}
		p.Run.ConcurrencyAdvice = 4
	})
	if _, note := Run(plan, false); strings.Contains(note.Text(), "并发建议") {
		t.Fatalf("建议值等于当前值不该留痕：%q", note.Text())
	}
	if plan.Run.Concurrency != 0 {
		t.Fatalf("不该改并发：%d", plan.Run.Concurrency)
	}
}

// 脚本后缀回退：非交互按确认放行并留痕。
func TestFallbackAdoptedUnderYes(t *testing.T) {
	plan := planOf(contract.ModeScript, func(p *contract.Plan) {
		p.Task.Script = &contract.ScriptTask{Path: "install.bin"}
		p.Run.Interpreter = "bash"
		p.Run.FallbackExt = ".bin"
	})
	r, note := Run(plan, false)
	if r != nil {
		t.Fatalf("非交互该按确认放行：%q", r.Say().Text())
	}
	if !strings.Contains(note.Text(), "未配解释器，直接回退到 bash") {
		t.Fatalf("留痕不对：%q", note.Text())
	}
}

// `--sudo` 撞 `--no-shell`：非交互上屏一行并留痕。
func TestNoShellDropsSudoUnderYes(t *testing.T) {
	plan := planOf(contract.ModeCommand, func(p *contract.Plan) {
		p.Task.Command = &contract.CommandTask{Text: "uptime"}
		p.Run.Sudo = true
		p.Run.NoShell = true
	})
	r, note := Run(plan, false)
	if r != nil {
		t.Fatalf("不该拒：%q", r.Say().Text())
	}
	if !strings.Contains(note.Text(), "--sudo 被 --no-shell 忽略") {
		t.Fatalf("留痕不对：%q", note.Text())
	}
}

// english 开着而代填值里有汉字：非交互直接取消（放行就是整批白跑）。
func TestChineseAnswerCancelledUnderYes(t *testing.T) {
	plan := planOf(contract.ModeCommand, func(p *contract.Plan) {
		p.Task.Command = &contract.CommandTask{
			Text:    "install.sh",
			Answers: []contract.Answer{{Value: "是", Triggers: []string{"continue"}}},
		}
		p.Run.English = true
	})
	r, note := Run(plan, false)
	if r == nil {
		t.Fatal("中文代填值配 english 应当取消")
	}
	if !strings.Contains(note.Text(), "english = true 与中文代填值同时给出") {
		t.Fatalf("留痕不对：%q", note.Text())
	}
}

// 触发词里有中文不算：服务端报文不受 english 影响，仍可能是中文。
func TestChineseTriggerIsFine(t *testing.T) {
	plan := planOf(contract.ModeCommand, func(p *contract.Plan) {
		p.Task.Command = &contract.CommandTask{
			Text:    "install.sh",
			Answers: []contract.Answer{{Value: "y", Triggers: []string{"是否继续"}}},
		}
		p.Run.English = true
	})
	if r, _ := Run(plan, false); r != nil {
		t.Fatalf("触发词里出现中文不该拦：%q", r.Say().Text())
	}
}
