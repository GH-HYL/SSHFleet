package output

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sshfleet/internal/batch"
	"sshfleet/internal/log"
	"sshfleet/internal/result"
	"sshfleet/internal/ssh"
)

// 运行期呈现器（结果的三去向 + 分类）回归。
//
// 这段行为原先住在 main 的闭包里，只能真跑一次 SSH 才能验证；收进 output 之后
// 三去向（终端 / output.txt / 执行期日志）与分类都成了可断言的。

// newTestReporter 造一个接住三个去向的呈现器：output.txt 与执行期日志都落到临时目录。
func newTestReporter(t *testing.T, mode string, total int) (*Reporter, *bytes.Buffer, string) {
	t.Helper()
	dir := t.TempDir()

	execLogPath := filepath.Join(dir, "SSHFleet.log")
	execLog, err := log.InitExec(dir, "SSHFleet.log")
	if err != nil {
		t.Fatalf("创建执行期日志失败：%v", err)
	}
	t.Cleanup(func() { _ = execLog.Close() })

	outBuf := &bytes.Buffer{}
	return NewReporter(execLog, outBuf, mode, total, nil), outBuf, execLogPath
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
		Seq: 0, IP: "10.0.0.1", ConnectSuccess: true, ExitCode: intp(0),
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

	logText := readExecLog(t, execLogPath)
	for _, want := range []string{"连接成功", "命令执行成功，退出码 0", "分类: 执行成功"} {
		if !strings.Contains(logText, want) {
			t.Fatalf("执行期日志缺少 %q：\n%s", want, logText)
		}
	}
}

// 连接失败：错误原文进分类明细，日志记「连接失败」；容器里没有退出码，不得出现退出码字样。
func TestReporterConnectFailure(t *testing.T) {
	r, outFile, execLogPath := newTestReporter(t, "execute", 1)

	res := ssh.Result{
		Seq: 0, IP: "10.0.0.2", ConnectSuccess: false, ConnectCostTime: 0.5,
		Error: ptr("dial tcp 10.0.0.2:22: connect: connection refused"),
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
		Seq: 0, IP: "10.0.0.3", ConnectSuccess: true, ExitCode: intp(0),
		ConnectCostTime: 0.1, ExecCostTime: 0.2,
		TotalFiles: 3, SuccessFiles: 3, FailedFiles: 0,
	}
	r.Result(res)

	if got := r.Category(res); got != result.SuccessCategoryTransport {
		t.Fatalf("传输模式成功分类应为 %q，实际 %q", result.SuccessCategoryTransport, got)
	}
	logText := readExecLog(t, execLogPath)
	if !strings.Contains(logText, "上传完成：成功 3/3 个文件") {
		t.Fatalf("执行期日志缺少上传完成记录：\n%s", logText)
	}

	// 有失败项时改走 WARN 级别与「（有失败项）」措辞
	r2, _, execLogPath2 := newTestReporter(t, "upload", 1)
	r2.Result(ssh.Result{
		Seq: 0, IP: "10.0.0.4", ConnectSuccess: true,
		TotalFiles: 3, SuccessFiles: 2, FailedFiles: 1,
		Error: ptr("b.txt: 上传失败 - 文件已存在"),
	})
	logText2 := readExecLog(t, execLogPath2)
	if !strings.Contains(logText2, "上传完成：成功 2/3 个文件（有失败项）") {
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

// 进度事件到达后才创建界面，且随后 Stop 会收尾（界面输出落在 stdout，这里只验证不 panic 且状态已建）。
func TestReporterProgressThenStop(t *testing.T) {
	r, _, _ := newTestReporter(t, "execute", 3)
	r.Progress(batch.Snapshot{Total: 3, Completed: 1, Succeeded: 1})
	r.mu.Lock()
	created := r.ui != nil
	r.mu.Unlock()
	if !created {
		t.Fatal("收到进度事件后应已创建进度界面")
	}
	r.Stop()
}
