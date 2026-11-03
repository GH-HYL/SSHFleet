package execute

// 执行期日志的开头一段（18 号单 A/B 补上的那块）：四种形态各一条。
//
// 期望串照老版同一次真跑抄下来——`build/建站05_18_2026-11-03_1445/out/*/old.history/SSHFleetExec.log`。
// 版式与措辞都在这一处钉住：脚本那一支的 stdin 行只说是哪份脚本（正文不进日志），
// 代填那一支先说表、再说「原始命令／原始脚本 → 处理方式 → 最终命令」。

import (
	"path/filepath"
	"strings"
	"testing"

	"sshfleet2/internal/contract"
	"sshfleet2/internal/log"
)

// captureSessionLog 跑一次 logSessionHead，交回整份日志正文。
func captureSessionLog(t *testing.T, p *contract.Plan, limit int) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "SSHFleetExec.log")
	lg, err := log.InitExec(path)
	if err != nil {
		t.Fatalf("开执行期日志失败：%v", err)
	}
	logSessionHead(lg, p, limit)
	if err := lg.Close(); err != nil {
		t.Fatalf("关执行期日志失败：%v", err)
	}
	return readAll(t, path)
}

// sessionPlan 造一张与老版那次真跑同形的纸（一台、root、英文、配了环境变量）。
func sessionPlan(mode contract.Mode) *contract.Plan {
	return &contract.Plan{
		Mode:  mode,
		Nodes: []contract.Node{{IP: "127.0.0.1", Port: "2201", User: "sshfleet"}},
		Run: contract.RunParams{
			Sudo:           true,
			Interpreter:    "bash",
			English:        true,
			Env:            map[string]string{"PATH": "/usr/local/bin:/usr/bin:/bin", "TERM": "dumb"},
			TimeoutConnect: 10,
			TimeoutExecute: 60,
			Downlink:       "sh -c 'export PATH=\"/usr/local/bin:/usr/bin:/bin\" TERM=\"dumb\"; sudo bash'",
		},
	}
}

// logBody 只留「消息正文」，去掉时间戳与级别——那两样每次不同。
func logBody(t *testing.T, text string) string {
	t.Helper()
	var b strings.Builder
	for _, ln := range strings.Split(text, "\n") {
		if strings.TrimSpace(ln) == "" {
			continue
		}
		// 形态：`2026-11-03 14:54:45.603  -  [INFO   ]  -  正文`
		parts := strings.SplitN(ln, "  -  ", 3)
		if len(parts) < 3 {
			t.Fatalf("日志行认不出：%q", ln)
		}
		b.WriteString(parts[2] + "\n")
	}
	return b.String()
}

// TestLogSessionCommand 命令模式：命令行 / stdin / 说明三行。
func TestLogSessionCommand(t *testing.T) {
	p := sessionPlan(contract.ModeCommand)
	p.Task.Command = &contract.CommandTask{Text: "echo hello | tr a-z A-Z"}
	p.Run.Stdin = "echo hello | tr a-z A-Z"

	want := "执行参数：执行超时 60 秒，连接超时 10 秒，执行身份 root\n" +
		"开始执行任务：节点 1 个，并发 1，模式 命令\n" +
		"  交给 SSH 执行：\n" +
		"    命令行： " + p.Run.Downlink + "\n" +
		"    stdin：  echo hello | tr a-z A-Z\n" +
		"    说明：   先导入配置文件里的环境变量，再判断目标机编码、把提示语钉成英文，然后把 stdin 送来的命令交给 root 身份的 bash（sudo）执行\n"

	if got := logBody(t, captureSessionLog(t, p, 1)); got != want {
		t.Fatalf("命令模式的开头一段与老版对不上\n got=%q\nwant=%q", got, want)
	}
}

// TestLogSessionScript 脚本模式：stdin 那一行只说是哪份脚本，正文不落日志；
// 说明那一句说的是「执行 stdin 送来的脚本」。
func TestLogSessionScript(t *testing.T) {
	p := sessionPlan(contract.ModeScript)
	p.Task.Script = &contract.ScriptTask{Path: "./scripts/hello.sh", Body: "echo hello-from-script\n"}
	p.Run.Stdin = p.Task.Script.Body
	p.Run.Downlink = "sh -c 'export PATH=\"/usr/local/bin:/usr/bin:/bin\"; sudo bash -c '\"'\"'. /dev/stdin'\"'\"' '\"'\"'hello.sh'\"'\"''"

	body := logBody(t, captureSessionLog(t, p, 1))
	for _, want := range []string{
		"    命令行： " + p.Run.Downlink + "\n",
		"    stdin：  脚本 ./scripts/hello.sh 的内容（bash 解释）\n",
		"    说明：   先导入配置文件里的环境变量，再判断目标机编码、把提示语钉成英文，然后以 root 身份执行 stdin 送来的脚本\n",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("脚本模式缺了 %q\n整份：%q", want, body)
		}
	}
	if strings.Contains(body, "echo hello-from-script") {
		t.Fatalf("脚本体不该进日志：%q", body)
	}
}

// TestLogSessionScriptAnswer 脚本＋代填：先说代填表，再交代原始脚本、处理方式与最终命令。
func TestLogSessionScriptAnswer(t *testing.T) {
	p := sessionPlan(contract.ModeScript)
	p.Task.Script = &contract.ScriptTask{
		Path: "ask1.sh", Body: "#!/bin/bash\nread -p \"Continue? \" a\n",
		Answers: []contract.Answer{{Value: "y", Triggers: []string{"Continue?", "Press enter to go on:"}}},
	}
	p.Run.Answer = "y,Continue?,Press enter to go on:"
	p.Run.Stdin = ""
	p.Run.Downlink = "sh -c '... sudo bash -c \"$(printf %s IyEvYmlu | base64 -d)\" '\"'\"'ask1.sh'\"'\"''"

	want := "代填表（1 条）：\n" +
		"  第 1 条：y → Continue?、Press enter to go on:\n" +
		"  交给 SSH 执行：\n" +
		"    原始脚本： ask1.sh（正文经 base64 编入命令行，不落盘）\n" +
		"    处理方式： 会话 stdin 整条让给代填（1 条）\n" +
		"    最终命令： " + p.Run.Downlink + "\n"

	if got := logBody(t, captureSessionLog(t, p, 1)); !strings.Contains(got, want) {
		t.Fatalf("脚本代填那一块与老版对不上\n want 含=%q\n got=%q", want, got)
	}
}

// TestLogSessionCommandAnswer 命令＋代填：与脚本那支只差「原始命令」与「处理方式」两行。
func TestLogSessionCommandAnswer(t *testing.T) {
	p := sessionPlan(contract.ModeCommand)
	p.Task.Command = &contract.CommandTask{
		Text:    `read -p "Continue? " a; echo answer=$a`,
		Answers: []contract.Answer{{Value: "y", Triggers: []string{"Continue?"}}},
	}
	p.Run.Answer = "y,Continue?"
	p.Run.Stdin = ""

	body := logBody(t, captureSessionLog(t, p, 1))
	want := "    原始命令： " + p.Task.Command.Text + "\n" +
		"    处理方式： 正文经 base64 编入命令行（不落盘），会话 stdin 整条让给代填（1 条）\n" +
		"    最终命令： " + p.Run.Downlink + "\n"
	if !strings.Contains(body, want) {
		t.Fatalf("命令代填那一块与老版对不上\n want 含=%q\n got=%q", want, body)
	}
	if strings.Contains(body, "交给 SSH 执行：\n    命令行：") {
		t.Fatalf("代填那一支不该写「命令行」：%q", body)
	}
}

// TestLogSessionUpload 上传：先交代被过滤的链接，再写两行；传输类的超时值是 300，
// 且没有「交给 SSH 执行」那一块。
func TestLogSessionUpload(t *testing.T) {
	p := sessionPlan(contract.ModeUpload)
	p.Run.Sudo = true
	p.Run.TimeoutTransfer = 300
	p.Task.Upload = &contract.UploadTask{Source: "./updir", Skipped: []string{"link.txt"}}

	want := "[提示] 上传源中有 1 个软链接/快捷方式被过滤（不上传）：link.txt\n" +
		"执行参数：执行超时 300 秒，连接超时 10 秒，执行身份 root\n" +
		"开始执行任务：节点 1 个，并发 1，模式 上传\n"

	if got := logBody(t, captureSessionLog(t, p, 1)); got != want {
		t.Fatalf("上传的开头一段与老版对不上\n got=%q\nwant=%q", got, want)
	}
}

// TestSplitLinesDropsTransferHeader 传输明细的统计头不进日志：见到 `total_files=` 前缀就丢，
// 不分模式也不分位置（老版 `splitLines` 就是这一条判据）。
func TestSplitLinesDropsTransferHeader(t *testing.T) {
	output := "total_files=2, success_files=0, failed_files=1\nlink.txt: 已跳过（符号链接）\none.txt: 上传失败\n"

	want := "link.txt: 已跳过（符号链接）\none.txt: 上传失败"
	if got := strings.Join(splitLines(output), "\n"); got != want {
		t.Fatalf("统计头没去掉\n got=%q\nwant=%q", got, want)
	}
	// 别的模式同样丢：老版这一条不分模式。
	if got := strings.Join(splitLines("第一名\ntotal_files=1"), "\n"); got != "第一名" {
		t.Fatalf("命令模式的统计头也该丢：%q", got)
	}
}
