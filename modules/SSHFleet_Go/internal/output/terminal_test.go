package output

import (
	"strings"
	"testing"

	"sshfleet/internal/ssh"
)

// 单条结果明细的字段顺序（用户 2026-09-15 裁定）：
// 连接 → 执行/错误 → 分类 → output 原文 → 分隔线。
// 终端与 output.txt 共用同一个 ResultLine，故一处断言覆盖两处。

func intPtr(v int) *int       { return &v }
func strPtr(v string) *string { return &v }

func lineIndex(lines []string, contains string) int {
	for i, ln := range lines {
		if strings.Contains(ln, contains) {
			return i
		}
	}
	return -1
}

func TestResultLineFieldOrderOnSuccess(t *testing.T) {
	got := ResultLine(ssh.Result{
		IP: "[10.0.0.1]", ConnectSuccess: true, ExitCode: intPtr(0),
		ConnectCostTime: 0.01, ExecCostTime: 0.02,
		Output: "hello world\nsecond line",
	}, "execute", "执行成功")

	lines := strings.Split(got, "\n")
	conn := lineIndex(lines, "连接: 成功")
	exec := lineIndex(lines, "执行: 成功")
	category := lineIndex(lines, "分类: 执行成功")
	output := lineIndex(lines, "hello world")
	sep := lineIndex(lines, strings.Repeat("=", 50))

	for name, idx := range map[string]int{"连接": conn, "执行": exec, "分类": category, "output": output, "分隔线": sep} {
		if idx < 0 {
			t.Fatalf("缺少 %s 行：\n%s", name, got)
		}
	}
	if !(conn < exec && exec < category && category < output && output < sep) {
		t.Fatalf("字段顺序应为 连接 < 执行 < 分类 < output < 分隔线，实际 %d,%d,%d,%d,%d：\n%s",
			conn, exec, category, output, sep, got)
	}
}

func TestResultLineFieldOrderOnConnectFailure(t *testing.T) {
	got := ResultLine(ssh.Result{
		IP: "[10.0.0.2]", ConnectSuccess: false, ConnectCostTime: 0.01,
		Error: strPtr("dial tcp: connection refused"),
	}, "execute", "拒绝网络连接")

	lines := strings.Split(got, "\n")
	conn := lineIndex(lines, "连接: 失败")
	errLine := lineIndex(lines, "错误: dial tcp")
	category := lineIndex(lines, "分类: 拒绝网络连接")
	sep := lineIndex(lines, strings.Repeat("=", 50))

	for name, idx := range map[string]int{"连接": conn, "错误": errLine, "分类": category, "分隔线": sep} {
		if idx < 0 {
			t.Fatalf("缺少 %s 行：\n%s", name, got)
		}
	}
	if !(conn < errLine && errLine < category && category < sep) {
		t.Fatalf("连接失败时顺序应为 连接 < 错误 < 分类 < 分隔线，实际 %d,%d,%d,%d：\n%s",
			conn, errLine, category, sep, got)
	}
	if strings.Contains(got, "执行:") {
		t.Fatalf("连接失败不应出现执行行：\n%s", got)
	}
}
