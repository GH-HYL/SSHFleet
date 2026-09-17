package output

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"sshfleet/internal/result"
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

// output 字段在呈现层**零处理**（用户 2026-09-15 裁定的分层）：整备——去掉整块首尾的
// 空白行——在采集侧做完（ssh.trimOuterBlankLines），ResultLine 拿到什么就输出什么。
// 所以行首缩进（`who -b` 的 system boot）与中间空行（`ls -l` 的分段）都原样带出。
// 终端与 output.txt 共用 ResultLine，此处断言同时覆盖两处去向。
func TestResultLinePassesOutputThrough(t *testing.T) {
	const raw = "         system boot  2026-09-15 10:23\n\ndisk  use%"
	got := ResultLine(ssh.Result{
		IP: "[10.0.0.1]", ConnectSuccess: true, ExitCode: intPtr(0),
		ConnectCostTime: 0.01, ExecCostTime: 0.02,
		Output: raw,
	}, "execute", "执行成功")

	if !strings.Contains(got, raw) {
		t.Fatalf("output 应原样输出（含行首缩进与中间空行），实际：\n%s", got)
	}
	if !strings.Contains(got, "disk  use%\n"+strings.Repeat("=", 50)) {
		t.Fatalf("output 末行与分隔线之间不应多出空行：\n%s", got)
	}
}

// 「提示：」块（2026-09-16）：内容取自配置文件里各分类的 tip 字段；
// 只有「执行失败(退出码N)」由工具按退出码补含义（分类名带数字，配置里写不了）。
func TestCategoryTipLines(t *testing.T) {
	kw, err := result.LoadKeywords(filepath.Join("..", "..", "config", "error_keywords.toml"))
	if err != nil {
		t.Fatalf("关键词文件应可加载: %v", err)
	}
	cats := []result.CategoryCount{
		{Category: "握手被断开", Count: 2},      // 配置里写了 tip
		{Category: "密码过期", Count: 1},       // 没写 tip → 不出现
		{Category: "执行失败(退出码1)", Count: 1}, // 内置说明 + 已知退出码含义
		{Category: "执行失败(退出码3)", Count: 1}, // 内置说明 + 未知退出码（无含义）
	}
	lines := categoryTipLines(cats, kw)

	if len(lines) != 3 {
		t.Fatalf("应出 3 行（没写 tip 的不算），实为 %d 行：%v", len(lines), lines)
	}
	// 配置里的 tip 原样跟出（不把文案抄进测试，改配置不用改测试）
	if want := "握手被断开：" + kw.TipOf("握手被断开"); lines[0] != want {
		t.Errorf("第 1 行应取自配置的 tip：\n  期望 %q\n  实际 %q", want, lines[0])
	}
	// 「执行失败(退出码N)」由工具补含义（已知码带含义，未知码只给前半句）
	for _, want := range []string{
		"执行失败(退出码1)：命令自身失败，或命令没跑起来被拒（未识别出具体原因）。退出码 1 = 一般性错误",
		"执行失败(退出码3)：命令自身失败，或命令没跑起来被拒（未识别出具体原因）",
	} {
		found := false
		for _, ln := range lines {
			if ln == want {
				found = true
			}
		}
		if !found {
			t.Errorf("缺少提示行：%q（实为 %v）", want, lines)
		}
	}
}

// 开关语义：开启时出「提示：」块、不再单独出「常见退出码」；
// 关闭时退回旧行为（只出「常见退出码」）。
func TestPrintStatisticsTipSwitch(t *testing.T) {
	kw, err := result.LoadKeywords(filepath.Join("..", "..", "config", "error_keywords.toml"))
	if err != nil {
		t.Fatalf("关键词文件应可加载: %v", err)
	}
	stats := &result.Stats{
		NodesTotal: 1, ResultsTotal: 1, Verify: "通过", FailCounts: 1,
		SortedFailCategories: []result.CategoryCount{
			{Category: "握手被断开", Count: 1},
			{Category: "执行失败(退出码1)", Count: 1},
		},
	}

	on := &bytes.Buffer{}
	PrintStatistics(on, stats, kw, true)
	if !strings.Contains(on.String(), "提示：") || !strings.Contains(on.String(), "握手被断开：") {
		t.Errorf("开启时应有提示块：\n%s", on.String())
	}
	if strings.Contains(on.String(), "常见退出码") {
		t.Errorf("开启时不应再单独出「常见退出码」（已并入提示块）：\n%s", on.String())
	}

	off := &bytes.Buffer{}
	PrintStatistics(off, stats, kw, false)
	if strings.Contains(off.String(), "提示：") {
		t.Errorf("关闭时不应有提示块：\n%s", off.String())
	}
	if !strings.Contains(off.String(), "常见退出码: 1 >> 一般性错误") {
		t.Errorf("关闭时应保留旧的「常见退出码」行：\n%s", off.String())
	}
}
