package output

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"sshfleet/internal/batch"
	"sshfleet/internal/log"
	"sshfleet/internal/result"
	"sshfleet/internal/ssh"
	"sshfleet/internal/verdict"
)

// 运行期呈现器（结果的三去向 + 分类）回归。
//
// 这段行为原先住在 main 的闭包里，只能真跑一次 SSH 才能验证；收进 output 之后
// 三去向（终端 / output.txt / 执行期日志）与分类都成了可断言的。

// newTestReporter 造一个接住三个去向的呈现器：output.txt 与执行期日志都落到临时目录。
func newTestReporter(t *testing.T, mode string, total int) (*Reporter, *bytes.Buffer, string) {
	t.Helper()
	return newTestReporterQuiet(t, mode, total, false)
}

// newTestReporterQuiet 同上，但可指定非交互模式（--yes 的静默闸门）。
func newTestReporterQuiet(t *testing.T, mode string, total int, quiet bool) (*Reporter, *bytes.Buffer, string) {
	t.Helper()
	dir := t.TempDir()

	execLogPath := filepath.Join(dir, "SSHFleet.log")
	execLog, err := log.InitExec(dir, "SSHFleet.log")
	if err != nil {
		t.Fatalf("创建执行期日志失败：%v", err)
	}
	t.Cleanup(func() { _ = execLog.Close() })

	outBuf := &bytes.Buffer{}
	return NewReporter(execLog, outBuf, mode, nil, total, nil, time.Now(), quiet), outBuf, execLogPath
}

// 非交互模式（--yes）下运行期提示只进日志、不上屏（L61 的静默总闸门）。
// 挡的只有"教学类"那一路——报错与结果明细照旧。
func TestNoticeQuietInNonInteractiveMode(t *testing.T) {
	// 非交互：终端那一路整个没有输出
	r, _, logPath := newTestReporterQuiet(t, "upload", 1, true)
	r.out = tempOut(t)
	r.Notice("提示：源里的软链接被过滤\n")
	if got := readTempOut(t, r.out); got != "" {
		t.Fatalf("非交互模式下提示不该上屏，实际输出：%q", got)
	}
	if !strings.Contains(readExecLog(t, logPath), "软链接被过滤") {
		t.Fatal("非交互模式下提示仍应进执行日志，否则事后无从追查")
	}

	// 交互模式照旧上屏
	r2, _, _ := newTestReporter(t, "upload", 1)
	r2.out = tempOut(t)
	r2.Notice("提示：源里的软链接被过滤\n")
	if got := readTempOut(t, r2.out); !strings.Contains(got, "软链接被过滤") {
		t.Fatalf("交互模式下提示应照常上屏，实际输出：%q", got)
	}
}

// tempOut 造一个可读回的"终端"文件（printAbove 默认写 os.Stdout，测试要把它换掉）。
func tempOut(t *testing.T) *os.File {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "stdout-*.txt")
	if err != nil {
		t.Fatalf("创建临时输出文件失败：%v", err)
	}
	t.Cleanup(func() { _ = f.Close() })
	return f
}

// readTempOut 读回 tempOut 写完的内容。
func readTempOut(t *testing.T, f *os.File) string {
	t.Helper()
	if err := f.Sync(); err != nil {
		t.Fatalf("刷新临时输出失败：%v", err)
	}
	data, err := os.ReadFile(f.Name())
	if err != nil {
		t.Fatalf("读回临时输出失败：%v", err)
	}
	return string(data)
}

// readExecLog 读执行期日志全文（呈现器写完即可读，Logger 每次写都直接落盘）。
func readExecLog(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读取执行期日志失败：%v", err)
	}
	return string(data)
}

func ptr(s string) *string { return &s }
func intp(i int) *int      { return &i }

// 命令模式：成功结果同时进 output.txt 与执行期日志，分类为「执行成功」。
func TestReporterResultWritesBothSinks(t *testing.T) {
	r, outFile, execLogPath := newTestReporter(t, "execute", 2)

	res := ssh.Result{
		Seq: 0, IP: "10.0.0.1", ConnectSuccess: true, Verdict: verdict.Success,
		ConnectCostTime: 0.123, ExecCostTime: 0.456, Output: "hello\n",
	}
	r.Result(res)

	txt := outFile.String()
	for _, want := range []string{"【10.0.0.1】", "连接: 成功 - 0.123s", "执行: 成功 - 0.456s", "分类: 执行成功"} {
		if !strings.Contains(txt, want) {
			t.Fatalf("output.txt 缺少 %q：\n%s", want, txt)
		}
	}
	// output.txt 的最后一行应是分隔线（结果之间一眼可分）
	if !strings.HasSuffix(strings.TrimRight(txt, "\n"), strings.Repeat("=", 50)) {
		t.Fatalf("output.txt 末行应为分隔线：\n%s", txt)
	}

	// 成功节点在执行期日志里只占一行（连接与执行合并）：1000+ 节点的正常执行
	// 不能靠逐节点铺行刷满日志；失败节点才展开（见 TestReporterLogsFailureEvidence）。
	logText := readExecLog(t, execLogPath)
	want := "成功：连接 0.123s，执行 0.456s"
	if !strings.Contains(logText, want) {
		t.Fatalf("执行期日志缺少 %q：\n%s", want, logText)
	}
	if lines := strings.Count(strings.TrimSpace(logText), "\n") + 1; lines != 1 {
		t.Fatalf("成功节点在执行期日志里应只占一行，实为 %d 行：\n%s", lines, logText)
	}
}

// 连接失败：错误原文进分类明细，日志记「连接失败」；容器里没有退出码，不得出现退出码字样。
func TestReporterConnectFailure(t *testing.T) {
	r, outFile, execLogPath := newTestReporter(t, "execute", 1)

	res := ssh.Result{
		Seq: 0, IP: "10.0.0.2", ConnectSuccess: false, Verdict: verdict.Other,
		Category: "dial tcp 10.0.0.2:22: connect: connection refused",
		ConnectCostTime: 0.5,
		Error:           ptr("dial tcp 10.0.0.2:22: connect: connection refused"),
	}
	r.Result(res)

	txt := outFile.String()
	if !strings.Contains(txt, "错误: dial tcp") {
		t.Fatalf("output.txt 应含错误原文：\n%s", txt)
	}
	if strings.Contains(txt, "退出码") {
		t.Fatalf("连接失败不该出现退出码（退出码语义见 CONTEXT）：\n%s", txt)
	}

	logText := readExecLog(t, execLogPath)
	if !strings.Contains(logText, "连接失败：dial tcp") {
		t.Fatalf("执行期日志应记连接失败原文：\n%s", logText)
	}
	// 原始错误文案本身作为兜底分类（关键词未命中时保留具体失败内容）
	if !strings.Contains(logText, "分类: dial tcp") {
		t.Fatalf("分类应为错误原文兜底：\n%s", logText)
	}
}

// 传输模式：分类走传输口径，上传完成按成功/失败文件数选级别。
func TestReporterTransferMode(t *testing.T) {
	r, outFile, execLogPath := newTestReporter(t, "upload", 1)

	res := ssh.Result{
		Seq: 0, IP: "10.0.0.3", ConnectSuccess: true, Verdict: verdict.Success,
		ConnectCostTime: 0.1, ExecCostTime: 0.2,
		TotalFiles: 3, SuccessFiles: 3, FailedFiles: 0,
	}
	r.Result(res)

	if got := displayCategory(res, "upload"); got != result.SuccessCategoryTransport {
		t.Fatalf("传输模式成功分类应为 %q，实际 %q", result.SuccessCategoryTransport, got)
	}
	logText := readExecLog(t, execLogPath)
	if !strings.Contains(logText, "成功：连接 0.100s，上传 3/3 个文件") {
		t.Fatalf("执行期日志缺少上传成功记录：\n%s", logText)
	}

	// 有失败项时改走 WARN 级别与「（有失败项）」措辞
	r2, _, execLogPath2 := newTestReporter(t, "upload", 1)
	r2.Result(ssh.Result{
		Seq: 0, IP: "10.0.0.4", ConnectSuccess: true,
		TotalFiles: 3, SuccessFiles: 2, FailedFiles: 1,
		Error: ptr("b.txt: 上传失败 - 文件已存在"),
	})
	logText2 := readExecLog(t, execLogPath2)
	if !strings.Contains(logText2, "上传完成：成功 2/3 个文件（有失败项 1 个）") {
		t.Fatalf("有失败项时应标「（有失败项）」：\n%s", logText2)
	}
	_ = outFile
}

// 采集期提示：界面尚未创建时直接落终端，并同时进执行期日志。
func TestReporterNoticeGoesToLog(t *testing.T) {
	r, _, execLogPath := newTestReporter(t, "upload", 1)
	r.Notice("提示：上传源中有 2 个软链接/快捷方式被过滤（不上传）：a.lnk, b.lnk")

	if !strings.Contains(readExecLog(t, execLogPath), "软链接/快捷方式被过滤") {
		t.Fatalf("采集期提示应写入执行期日志")
	}
}

// 进度界面懒创建：没收到进度事件时 Stop 是空操作（不该崩、不该写东西）。
func TestReporterStopWithoutProgress(t *testing.T) {
	r, outFile, _ := newTestReporter(t, "execute", 1)
	r.Stop()
	r.Stop() // 幂等
	if outFile.Len() != 0 {
		t.Fatalf("未创建进度界面时 Stop 不该写任何东西，实际：%q", outFile.String())
	}
}

// 直通模式（输出不是终端，如 go test 的管道）：进度事件不打进度条，也不该崩；Stop 幂等。
// 真终端下会启动 bubbletea 界面——那条路径走不到这里（go test 的 stdout 是管道），
// 由手工跑一次真实执行覆盖。
func TestReporterProgressInPlainModeWhenNotTTY(t *testing.T) {
	r, _, _ := newTestReporter(t, "execute", 3)
	if !r.plain {
		t.Skip("当前 go test 的输出是终端，本用例只覆盖直通模式")
	}

	r.Progress(batch.Snapshot{Total: 3, Completed: 1, Succeeded: 1})
	r.mu.Lock()
	created := r.prog != nil
	r.mu.Unlock()
	if created {
		t.Fatal("直通模式下不该启动进度界面")
	}
	r.Stop()
}

// TTY 路径：进度界面能启动、能收得回来。
// go test 的 stdout 是管道（所以上面的用例永远走直通），这里把 plain 关掉、
// 输出改指临时文件，让 bubbletea 真跑一遍——渲染内容会落进那个文件。
func TestReporterRunsProgressProgram(t *testing.T) {
	r, _, _ := newTestReporter(t, "execute", 2)

	f, err := os.CreateTemp(t.TempDir(), "progress-*.out")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	r.out = f
	r.plain = false

	r.Progress(batch.Snapshot{Total: 2, Completed: 1, Succeeded: 1})
	r.mu.Lock()
	started := r.prog != nil
	r.mu.Unlock()
	if !started {
		t.Fatal("关掉直通模式后，首个进度事件应启动进度界面")
	}

	r.Stop() // 会等渲染收尾，因此之后文件里必有内容

	info, err := os.Stat(f.Name())
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() == 0 {
		t.Fatal("进度界面应至少渲染过一次")
	}
}

// 执行期日志要留下「为什么失败」的证据（用户 2026-09-16：异常时的报文此前全丢）。
//
// 两条通道此前都是空的：Error 原文只在连接失败时写过，Output 则从不进日志。
// 前者是「连上了却没跑成」的唯一原因来源（创建会话失败 / 超时 / 路径不存在），
// 后者是服务端拒绝语与逐文件失败原因的唯一来源（密码过期、nologin、传输明细）。
func TestReporterLogsFailureEvidence(t *testing.T) {
	// 1) 命令失败：输出明细进日志（密码过期就是这种：Error 为空、证据在 Output）
	r, _, logPath := newTestReporter(t, "execute", 1)
	r.Result(ssh.Result{
		Seq: 0, IP: "10.0.0.9", User: "root", AuthMethod: "密码",
		ConnectSuccess: true, ExitCode: intp(1), ConnectCostTime: 0.1, ExecCostTime: 0.004,
		Output: "WARNING: Your password has expired.\nPassword change required but no TTY available.",
	})
	logText := readExecLog(t, logPath)
	for _, want := range []string{
		"连接成功，用户 root，登录方式 密码",
		"命令执行失败，退出码 1",
		"输出明细（2 行）：",
		"WARNING: Your password has expired.",
		"Password change required but no TTY available.",
	} {
		if !strings.Contains(logText, want) {
			t.Fatalf("执行期日志缺少 %q：\n%s", want, logText)
		}
	}

	// 2) 连上了但没跑成：报错原文进日志（Error 非空、退出码缺席）
	r2, _, logPath2 := newTestReporter(t, "execute", 1)
	r2.Result(ssh.Result{
		Seq: 0, IP: "10.0.0.10", ConnectSuccess: true, ConnectCostTime: 0.2,
		Error: ptr("创建会话失败 - ssh: rejected: connect failed (\"open failed\")"),
	})
	logText2 := readExecLog(t, logPath2)
	if !strings.Contains(logText2, "错误详情：创建会话失败") {
		t.Fatalf("执行期日志应记下失败原因原文：\n%s", logText2)
	}

	// 3) 成功节点不写输出明细（`cat 大文件` 这类命令不能把日志撑爆）
	r3, _, logPath3 := newTestReporter(t, "execute", 1)
	r3.Result(ssh.Result{
		Seq: 0, IP: "10.0.0.11", ConnectSuccess: true, Verdict: verdict.Success,
		ConnectCostTime: 0.1, ExecCostTime: 0.2, Output: "line1\nline2\nline3",
	})
	if strings.Contains(readExecLog(t, logPath3), "输出明细") {
		t.Fatalf("成功节点不该写输出明细：\n%s", readExecLog(t, logPath3))
	}

	// 4) 超长输出封顶：只写上限行数，其余报数并指向文件
	var big strings.Builder
	for i := 0; i < maxOutputLines+7; i++ {
		fmt.Fprintf(&big, "line %d\n", i)
	}
	r4, _, logPath4 := newTestReporter(t, "execute", 1)
	r4.Result(ssh.Result{
		Seq: 0, IP: "10.0.0.12", ConnectSuccess: true, ExitCode: intp(2),
		ConnectCostTime: 0.1, ExecCostTime: 0.2, Output: big.String(),
	})
	logText4 := readExecLog(t, logPath4)
	if !strings.Contains(logText4, "其余 7 行省略") {
		t.Fatalf("超出上限应报省略行数：\n%s", logText4)
	}
	if strings.Contains(logText4, fmt.Sprintf("line %d", maxOutputLines)) {
		t.Fatalf("第 %d 行不该写入（已超上限）：\n%s", maxOutputLines, logText4)
	}
}

// D35：执行期日志的「错误详情：」也要带出未送出的代填——失败行该显示失败详情的地方
// 口径一致（终端明细 / output.txt / 执行期日志 / 两张 xlsx），一处合成、处处一样。
func TestReporterLogsMissedAnswers(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "SSHFleet.log")
	execLog, err := log.InitExec(dir, "SSHFleet.log")
	if err != nil {
		t.Fatalf("创建执行期日志失败：%v", err)
	}
	t.Cleanup(func() { _ = execLog.Close() })

	answers := []ssh.Answer{{Value: "zhangsan", Triggers: []string{"Your full name"}}}
	r := NewReporter(execLog, &bytes.Buffer{}, "execute", answers, 1, nil, time.Now(), true)
	zero := 0
	r.Result(ssh.Result{
		Seq: 0, IP: "10.0.0.7", ConnectSuccess: true, Verdict: verdict.Other,
		Category: "触发词未命中", ExitCode: &zero,
		ConnectCostTime: 0.1, ExecCostTime: 6.07,
		AnswersMissed: []int{1}, Output: "name= dept=",
	})

	logText := readExecLog(t, logPath)
	if !strings.Contains(logText, `错误详情：未送出的代填：第 1 条（触发词 "Your full name"）`) {
		t.Fatalf("执行期日志应带未送出的代填：\n%s", logText)
	}
	if !strings.Contains(logText, "命令执行失败，退出码 0") {
		t.Fatalf("退出码 0 却失败（成败与退出码解绑）应照旧记下：\n%s", logText)
	}
	if !strings.Contains(logText, "分类: 触发词未命中") {
		t.Fatalf("分类行照旧由分类列表达：\n%s", logText)
	}
}

// W10：中间步骤的退出码在失败行可见——回答「工具在远端额外跑了什么、结果如何」。
// 与 D35 同一条管道：errText 一处合成，终端 / output.txt / 执行期日志 / 两张 xlsx 处处一样。
func TestReporterLogsSteps(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "SSHFleet.log")
	execLog, err := log.InitExec(dir, "SSHFleet.log")
	if err != nil {
		t.Fatalf("创建执行期日志失败：%v", err)
	}
	t.Cleanup(func() { _ = execLog.Close() })

	one := 1
	r := NewReporter(execLog, &bytes.Buffer{}, "download", nil, 1, nil, time.Now(), true)
	r.Result(ssh.Result{
		Seq: 0, IP: "10.0.0.8", ConnectSuccess: true, Verdict: verdict.Other,
		Category: "远程路径不存在", ExitCode: nil,
		ConnectCostTime: 0.1, ExecCostTime: 0.3,
		Error: ptr("远程路径不存在: /home/u/missing.txt"),
		Steps: []ssh.StepResult{{Name: ssh.StepPrecheck, ExitCode: &one}},
	})

	logText := readExecLog(t, logPath)
	if !strings.Contains(logText, "中间步骤：下载预检（test -e） 退出码 1") {
		t.Fatalf("执行期日志应带中间步骤详情：\n%s", logText)
	}
	if !strings.Contains(logText, "错误详情：远程路径不存在") {
		t.Fatalf("报错原文段应在前：\n%s", logText)
	}
}

// stepsNote 本体：语义名映射中文、未知名原样兜底不猜、退出码缺席不推 0。
func TestStepsNote(t *testing.T) {
	one := 1
	got := stepsNote(ssh.Result{Steps: []ssh.StepResult{
		{Name: ssh.StepPasswd, ExitCode: &one},
		{Name: "mystery", ExitCode: nil},
	}})
	if want := "中间步骤：改密收尾（:） 退出码 1、mystery 未拿到退出码"; got != want {
		t.Fatalf("stepsNote 渲染不对：\n got: %s\nwant: %s", got, want)
	}
	if stepsNote(ssh.Result{}) != "" {
		t.Fatalf("无步骤时不应产出任何文本")
	}
}

// DisplayCommand：报告与工具日志里的命令行必须能照抄重跑。
//
// argv 里早没了引号（本机 shell 剥掉了），平铺拼接会让 `-c 'who -b'` 变成
// `-c who -b`——那是另一条命令，`-b` 会被当成多余参数。
func TestDisplayCommand(t *testing.T) {
	cases := []struct {
		name string
		argv []string
		want string
	}{
		{
			name: "带空格的命令补回引号",
			argv: []string{"SSHFleet.exe", "-f", "nodes.csv", "-c", "who -b"},
			want: "SSHFleet.exe -f nodes.csv -c 'who -b'",
		},
		{
			name: "纯单字参数不加引号",
			argv: []string{"SSHFleet.exe", "-f", "nodes.csv", "-c", "pwd"},
			want: "SSHFleet.exe -f nodes.csv -c pwd",
		},
		{
			name: "内联清单的逗号不加引号",
			argv: []string{"SSHFleet.exe", "-f", "172.28.118.49,22,root", "-c", "id"},
			want: "SSHFleet.exe -f 172.28.118.49,22,root -c id",
		},
		{
			name: "带空格的路径补回引号",
			argv: []string{"SSHFleet.exe", "-u", "D:/My Dir/a.txt", "-p", "/opt/"},
			want: "SSHFleet.exe -u 'D:/My Dir/a.txt' -p /opt/",
		},
		{
			name: "空参数给一对引号（不留看不见的空位）",
			argv: []string{"SSHFleet.exe", "-k"},
			want: "SSHFleet.exe -k",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := DisplayCommand(c.argv); got != c.want {
				t.Fatalf("应为 %q，实际 %q", c.want, got)
			}
		})
	}
}
