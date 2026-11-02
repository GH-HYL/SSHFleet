package message

// 结果行的版式：逐字写死期望串。
//
// 这些串是按老版 `output/terminal.go` 的 `ResultLine` 抄下来的版式（字段顺序 2026-09-15 裁定
// 过），不是「跑一遍看看出来什么」。

import (
	"strings"
	"testing"
	"time"

	"sshfleet2/internal/contract"
)

// TestResultLineSuccess 干净成功：连接、执行、分类、原文、分隔线，五行。
func TestResultLineSuccess(t *testing.T) {
	res := contract.NodeResult{
		IP:          "10.0.0.1",
		Connected:   true,
		ConnectCost: 123 * time.Millisecond,
		Cost:        456 * time.Millisecond,
		Category:    "执行成功",
		Output:      "uid=0(root) gid=0(root)",
	}

	want := "【10.0.0.1】 连接: 成功 - 0.123s\n" +
		"【10.0.0.1】 执行: 成功 - 0.456s\n" +
		"【10.0.0.1】 分类: 执行成功\n" +
		"uid=0(root) gid=0(root)\n" +
		"==================================================\n"

	if got := ResultLine(res, contract.ModeCommand).Text(); got != want {
		t.Fatalf("成功行不对。\n期望：\n%s\n实际：\n%s", want, got)
	}
}

// TestResultLineFailure 连上了却失败：多一行「错误详情」，分类是判据表给的名字。
func TestResultLineFailure(t *testing.T) {
	code := 3
	res := contract.NodeResult{
		IP:          "10.0.0.1",
		Connected:   true,
		ConnectCost: 123 * time.Millisecond,
		Cost:        456 * time.Millisecond,
		Category:    "执行失败(退出码3)",
		ExitCode:    &code,
		ErrorText:   "sshfleet.run.rejected: rejected by remote",
		Output:      "boom",
	}

	want := "【10.0.0.1】 连接: 成功 - 0.123s\n" +
		"【10.0.0.1】 执行: 失败 - 0.456s\n" +
		"【10.0.0.1】 错误详情: sshfleet.run.rejected: rejected by remote\n" +
		"【10.0.0.1】 分类: 执行失败(退出码3)\n" +
		"boom\n" +
		"==================================================\n"

	if got := ResultLine(res, contract.ModeCommand).Text(); got != want {
		t.Fatalf("失败行不对。\n期望：\n%s\n实际：\n%s", want, got)
	}
}

// TestResultLineConnectFailure 连不上：只两行事实加分类，不印原文那一格（那一格本来就是空的）。
func TestResultLineConnectFailure(t *testing.T) {
	res := contract.NodeResult{
		IP:          "10.0.0.2",
		ConnectCost: 11 * time.Millisecond,
		Category:    "连接失败",
		ErrorText:   "sshfleet.connect.refused: dial tcp 10.0.0.2:22: connect: connection refused",
	}

	want := "【10.0.0.2】 连接: 失败 - 0.011s\n" +
		"【10.0.0.2】 错误: sshfleet.connect.refused: dial tcp 10.0.0.2:22: connect: connection refused\n" +
		"【10.0.0.2】 分类: 连接失败\n" +
		"==================================================\n"

	if got := ResultLine(res, contract.ModeCommand).Text(); got != want {
		t.Fatalf("连接失败行不对。\n期望：\n%s\n实际：\n%s", want, got)
	}
}

// TestResultLineShowsControlChars 远端回来的控制字符换成看得见的图示，不交给终端当指令。
func TestResultLineShowsControlChars(t *testing.T) {
	res := contract.NodeResult{
		IP:        "10.0.0.1",
		Connected: true,
		Category:  "执行成功",
		Output:    "one\rtwo",
	}
	want := "【10.0.0.1】 连接: 成功 - 0.000s\n" +
		"【10.0.0.1】 执行: 成功 - 0.000s\n" +
		"【10.0.0.1】 分类: 执行成功\n" +
		"one␍two\n" +
		"==================================================\n"
	if got := ResultLine(res, contract.ModeCommand).Text(); got != want {
		t.Fatalf("控制字符没换成图示。实际：\n%s", got)
	}
}

// TestResultLineUnknownError 连接失败、原文又空：那一格给兜底的那句，不留空白。
func TestResultLineUnknownError(t *testing.T) {
	res := contract.NodeResult{IP: "10.0.0.3", Category: "原因未知"}
	got := ResultLine(res, contract.ModeCommand).Text()
	if !strings.Contains(got, "【10.0.0.3】 错误: 未知错误\n") {
		t.Fatalf("原文为空时该给「未知错误」，实际：\n%s", got)
	}
}
