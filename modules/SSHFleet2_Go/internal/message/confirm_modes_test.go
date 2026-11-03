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

	got := stripANSI(ConfirmScreen(p, "ok.csv", "").Text())
	if !strings.Contains(got, "▶ 脚本路径-→   deploy.sh") {
		t.Fatalf("脚本屏没有脚本路径：\n%s", got)
	}
	if strings.Contains(got, "解释器") {
		t.Fatalf("解释器是默认的 bash 时不该占一行：\n%s", got)
	}

	p.Run.Interpreter = "python3"
	got = stripANSI(ConfirmScreen(p, "ok.csv", "").Text())
	if !strings.Contains(got, "▶ 解释器  -→   python3") {
		t.Fatalf("解释器不是 bash 时要占一行：\n%s", got)
	}
}

// --no-shell 下解释器不参与执行：那一行的值改成提示。
func TestConfirmScreenInterpreterLineForNoShell(t *testing.T) {
	p := plainPlan(contract.ModeCommand)
	p.Task.Command = &contract.CommandTask{Text: "uptime"}
	p.Run.Interpreter = "python3"
	p.Run.NoShell = true

	got := stripANSI(ConfirmScreen(p, "ok.csv", "").Text())
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

	got := stripANSI(ConfirmScreen(p, "ok.csv", "").Text())
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

	got := stripANSI(ConfirmScreen(p, "ok.csv", "").Text())
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

	got := stripANSI(ConfirmScreen(p, "ok.csv", "N********").Text())
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
	got := stripANSI(ConfirmScreen(p, "ok.csv", "").Text())
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
