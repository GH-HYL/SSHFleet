package message

// 确认屏的四支（建站 05 的 09 号单）：脚本、上传、下载、改密各摆各的内容。
//
// 期望串照老版 `confirm/confirm.go` 的 `buildInfoTable` 的形态录：上色码剥掉再比，
// 这一条管「哪几行、什么字、对齐没有」；颜色对不对归 A/B 与老二进制逐字节对。

import (
	"strings"
	"testing"

	"sshfleet2/internal/contract"
)

func plainPlan(mode contract.Mode) *contract.Plan {
	return &contract.Plan{
		Mode:      mode,
		Resources: contract.Resources{Manifest: "ok.csv"},
		Nodes:     []contract.Node{{IP: "10.0.0.1"}},
		Run:       contract.RunParams{TimeoutConnect: 10, TimeoutExecute: 60, Remark: "r"},
	}
}

// 脚本屏：脚本路径 + 解释器那一行（非 bash 才占行）。
func TestConfirmScreenScript(t *testing.T) {
	p := plainPlan(contract.ModeScript)
	p.Task.Script = &contract.ScriptTask{Path: "deploy.sh"}
	p.Run.Interpreter = "bash"

	got := stripANSI(ConfirmScreen(p, "ok.csv", "", false).Text())
	if !strings.Contains(got, "▶ 脚本路径-→   deploy.sh") {
		t.Fatalf("脚本屏没有脚本路径：\n%s", got)
	}
	if strings.Contains(got, "解释器") {
		t.Fatalf("解释器是默认的 bash 时不该占一行：\n%s", got)
	}

	p.Run.Interpreter = "python3"
	got = stripANSI(ConfirmScreen(p, "ok.csv", "", false).Text())
	if !strings.Contains(got, "▶ 解释器  -→   python3") {
		t.Fatalf("解释器不是 bash 时要占一行：\n%s", got)
	}
}

// 本机路径写法不算 bash：解释器跑在目标机上，`\` 不是它的分隔符。
//
// 这一格曾经是两处两结论：显示侧把它当成 bash（省掉这一行），解析站归不出来
// （`-a` 那条判据说「不支持解释器」）。现在两处按同一条判：照实印出原值、也照实说不认。
func TestConfirmScreenInterpreterWindowsPathIsNotBash(t *testing.T) {
	p := plainPlan(contract.ModeScript)
	p.Task.Script = &contract.ScriptTask{Path: "deploy.sh"}
	p.Run.Interpreter = `C:\Python\bash`

	got := stripANSI(ConfirmScreen(p, "ok.csv", "", false).Text())
	if !strings.Contains(got, "▶ 解释器  -→   C:\\Python\\bash") {
		t.Fatalf("归不出 bash 时这一行要照实印出配置里的原值：\n%s", got)
	}
}

// --no-shell 下解释器不参与执行：那一行的值改成提示。
func TestConfirmScreenInterpreterLineForNoShell(t *testing.T) {
	p := plainPlan(contract.ModeCommand)
	p.Task.Command = &contract.CommandTask{Text: "uptime"}
	p.Run.Interpreter = "python3"
	p.Run.NoShell = true

	got := stripANSI(ConfirmScreen(p, "ok.csv", "", false).Text())
	if !strings.Contains(got, "原样下发，本次不生效") {
		t.Fatalf("--no-shell 时解释器行要换成提示：\n%s", got)
	}
}

// 上传屏：执行身份、本地与远程路径，代填行与上传内容树不在这张表里。
func TestConfirmScreenUpload(t *testing.T) {
	p := plainPlan(contract.ModeUpload)
	p.Task.Upload = &contract.UploadTask{Source: "./dist"}
	p.Task.Path = "/opt/app/"
	p.Run.Sudo = true // 出厂配置 sudo = true

	got := stripANSI(ConfirmScreen(p, "ok.csv", "", false).Text())
	for _, want := range []string{
		"▶ 执行身份-→   root", // 出厂配置 sudo = true
		"▶ 执行模式-→   上传模式",
		"▶ 本地路径-→   ./dist",
		"▶ 远程路径-→   /opt/app/",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("上传屏少了 %q：\n%s", want, got)
		}
	}
}

// 下载屏：远程在前、本地在后。
func TestConfirmScreenDownload(t *testing.T) {
	p := plainPlan(contract.ModeDownload)
	p.Task.Download = &contract.DownloadTask{RemotePath: "/var/log/app"}
	p.Task.Path = "./logs"

	got := stripANSI(ConfirmScreen(p, "ok.csv", "", false).Text())
	remote := strings.Index(got, "▶ 远程路径-→   /var/log/app")
	local := strings.Index(got, "▶ 本地路径-→   ./logs")
	if remote < 0 || local < 0 || remote > local {
		t.Fatalf("下载屏的次序不对（远程要在本地之前）：\n%s", got)
	}
}

// 改密屏：新密码只印遮过的形态。
func TestConfirmScreenPasswd(t *testing.T) {
	p := plainPlan(contract.ModePasswd)
	p.Task.Passwd = &contract.PasswdTask{Raw: "NewPass#1", NewPassword: "NewPass#1"}

	got := stripANSI(ConfirmScreen(p, "ok.csv", "N********", false).Text())
	if !strings.Contains(got, "▶ 新密码  -→   N********") {
		t.Fatalf("改密屏没有遮过的新密码：\n%s", got)
	}
	if strings.Contains(got, "NewPass#1") {
		t.Fatalf("明文新密码上了屏：\n%s", got)
	}
}

// 有代填就多几行「代填 N：内容 → 触发词」，内容为空时写「（回车）」。
func TestConfirmScreenAnswerRows(t *testing.T) {
	p := plainPlan(contract.ModeCommand)
	p.Task.Command = &contract.CommandTask{
		Text: "install.sh",
		Answers: []contract.Answer{
			{Value: "y", Triggers: []string{"是否继续", "continue"}},
			{Value: "", Triggers: []string{"按回车"}},
		},
	}
	got := stripANSI(ConfirmScreen(p, "ok.csv", "", false).Text())
	for _, want := range []string{
		"代填 1", "→   y → 是否继续、continue",
		"代填 2", "→   （回车） → 按回车",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("代填行不对，缺 %q：\n%s", want, got)
		}
	}
}

// 上传内容树：一层，目录带斜杠，末尾那个用 └──。
func TestUploadTree(t *testing.T) {
	p := plainPlan(contract.ModeUpload)
	p.Task.Upload = &contract.UploadTask{
		Source:     "/home/ops/dist",
		RootIsDir:  true,
		TopEntries: []contract.UploadEntry{{Name: "app", IsDir: true}, {Name: "run.sh"}, {Name: "link.txt"}},
	}
	got := stripANSI(UploadTree(p).Text())
	want := "\n📁 上传文件/目录内容 (-u 选项):\n" +
		"└── dist/\n" +
		"    ├── app/\n" +
		"    ├── run.sh\n" +
		"    └── link.txt\n"
	if got != want {
		t.Fatalf("内容树不对\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

// 单文件上传：根就是那个文件，一行 `(文件)`。
func TestUploadTreeSingleFile(t *testing.T) {
	p := plainPlan(contract.ModeUpload)
	p.Task.Upload = &contract.UploadTask{Source: "/home/ops/app.bin"}

	got := stripANSI(UploadTree(p).Text())
	if !strings.Contains(got, "└── app.bin (文件)\n") {
		t.Fatalf("单文件那棵树不对：\n%s", got)
	}
}

// 大小写成人话：1024 进制、一位小数。
func TestBytes(t *testing.T) {
	cases := map[int64]string{
		512:                  "512 B",
		1536:                 "1.5 KB",
		12 * 1024 * 1024 / 1: "12.0 MB",
	}
	for n, want := range cases {
		if got := Bytes(n); got != want {
			t.Errorf("Bytes(%d) = %q，want %q", n, got, want)
		}
	}
}

// 超时那一行：值按模式取，措辞也按模式取。上传下载印的是 `timeout_transfer` 那一格
// （默认 300），命令/脚本/改密印 `timeout_execute`（默认 60）。
//
// 期望串照老版确认屏录：`build/建站05_18_2026-11-03_1445/out/confirm-*-old.txt`。
// 这一行此前一律读 `TimeoutExecute`，上传下载屏因此印成 60s——09 号单的屏断言没盖到它，
// 18 号单跑 A/B 才露出来。
func TestConfirmScreenTimeoutRowPerMode(t *testing.T) {
	cases := []struct {
		mode contract.Mode
		task func(*contract.Plan)
		want string
	}{
		{
			mode: contract.ModeCommand,
			task: func(p *contract.Plan) { p.Task.Command = &contract.CommandTask{Text: "uptime"} },
			want: "▶ 执行超时-→   60s",
		},
		{
			mode: contract.ModeScript,
			task: func(p *contract.Plan) { p.Task.Script = &contract.ScriptTask{Path: "deploy.sh"} },
			want: "▶ 执行超时-→   60s",
		},
		{
			mode: contract.ModeUpload,
			task: func(p *contract.Plan) { p.Task.Upload = &contract.UploadTask{Source: "./dist"} },
			want: "▶ 传输超时-→   300s",
		},
		{
			mode: contract.ModeDownload,
			task: func(p *contract.Plan) { p.Task.Download = &contract.DownloadTask{RemotePath: "/var/log/app"} },
			want: "▶ 传输超时-→   300s",
		},
		{
			mode: contract.ModePasswd,
			task: func(p *contract.Plan) { p.Task.Passwd = &contract.PasswdTask{NewPassword: "NewPass#1"} },
			want: "▶ 改密超时-→   60s",
		},
	}
	for _, c := range cases {
		p := plainPlan(c.mode)
		p.Run.TimeoutExecute = 60
		p.Run.TimeoutTransfer = 300
		c.task(p)

		got := stripANSI(ConfirmScreen(p, "ok.csv", "", false).Text())
		if !strings.Contains(got, c.want) {
			t.Fatalf("%s：超时那一行该是 %q\n整屏：\n%s", modeTitle(c.mode), c.want, got)
		}
	}
}

// 采纳并发建议之后重显的那一屏：并发数那行加粗并注明「已按建议修改」（ADR-0040 决定三）。
//
// 期望串照老版：`ESC[1m` + `ESC[38;5;214m` + 值 + `ESC[0m`——加粗在橙色之前，只一层，
// 外面不再套表格那层橙（叠两层转义序列就与老版差一截）。
func TestConfirmScreenConcurrencyAdopted(t *testing.T) {
	p := plainPlan(contract.ModeUpload)
	p.Task.Upload = &contract.UploadTask{Source: "./dist"}
	p.Nodes = []contract.Node{{IP: "10.0.0.1"}, {IP: "10.0.0.2"}}
	p.Run.Concurrency = 2

	adopted := ConfirmScreen(p, "ok.csv", "", true).Text()
	if want := "\x1b[1m\x1b[38;5;214m2（已按建议修改）\x1b[0m"; !strings.Contains(adopted, want) {
		t.Fatalf("采纳之后并发数那行该是 %q\n整屏（带转义）：%q", want, adopted)
	}

	plain := ConfirmScreen(p, "ok.csv", "", false).Text()
	if strings.Contains(plain, "已按建议修改") {
		t.Fatalf("没采纳时不该有那句注：%q", plain)
	}
}
