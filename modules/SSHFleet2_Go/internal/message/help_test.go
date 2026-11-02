package message

import (
	"strings"
	"testing"
)

// 宽度判据用写死的字面量，不用被测代码算——期望值独立于实现（规范三.3）。

func TestRuneWidthLiterals(t *testing.T) {
	cases := []struct {
		in   string
		want int
	}{
		{"abc", 3},
		{"中文", 4},
		{"（」", 4},      // 全角标点占 2 列
		{"a\uFE0F", 1}, // 变体选择符算 0 列
		{"a\u0301", 1}, // 组合记号算 0 列
		{"", 0},
		{"a​​b", 2}, // 零宽空格（Cf）不算列
	}
	for _, c := range cases {
		if got := displayWidth(c.in); got != c.want {
			t.Errorf("displayWidth(%q) = %d，want %d", c.in, got, c.want)
		}
	}
}

func TestTrimToWidth(t *testing.T) {
	if got := trimToWidth("中文abc", 5); got != "中文a" {
		t.Errorf("trimToWidth 保守取前缀：got %q", got)
	}
	if got := trimToWidth("abc", 10); got != "abc" {
		t.Errorf("放得下就全留：got %q", got)
	}
	if got := trimToWidth("中文", 1); got != "" {
		t.Errorf("第一个字符就放不下时应返回空：got %q", got)
	}
}

func TestWrapByWidth(t *testing.T) {
	if got := wrapByWidth("一条不算短的说明文字", 0); len(got) != 1 {
		t.Errorf("宽度 0 不折行：got %v", got)
	}
	if got := wrapByWidth("短", 0); len(got) != 1 {
		t.Errorf("短文本宽度 0 也不折：got %v", got)
	}

	// 在可断处断开：全角标点之后（「第X段，」正好 8 列，末段不足一行）
	got := wrapByWidth("第一段，第二段，第三段，第四段", 8)
	want := []string{"第一段，", "第二段，", "第三段，", "第四段"}
	if len(got) != len(want) {
		t.Fatalf("应折成 %d 行，got %v", len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("第 %d 行 want %q，got %q", i, want[i], got[i])
		}
	}

	// 行内没有断点就硬断
	got = wrapByWidth("abcdefghij", 4)
	if len(got) != 3 || got[0] != "abcd" || got[1] != "efgh" || got[2] != "ij" {
		t.Errorf("硬断按宽度切：got %v", got)
	}

	// 行尾空白裁掉
	got = wrapByWidth("aa bb cc dd", 5)
	if got[0] != "aa bb" {
		t.Errorf("行尾空格应裁掉：got %q", got)
	}

	// 单字符超宽不死循环（宽度 1、全角字符）：保底每行推进一个字符
	got = wrapByWidth("中文", 1)
	if len(got) != 2 || got[0] != "中" || got[1] != "文" {
		t.Errorf("保底每行推进一个字符：got %v", got)
	}
}

// 木偶数据：一条组标题、一条空行、一条长说明的选项——结构断言与排版断言都用它。
func fixtureEntries() []HelpEntry {
	return []HelpEntry{
		{Kind: EntryGroup, Desc: "一组"},
		{Kind: EntryBlank},
		{Kind: EntryOption, Short: "-x", Long: "--example", Desc: "这是一条相当长的说明，专门用来验证折行不串列、也不越出终端宽度"},
	}
}

// descStart 木偶数据的说明列起点：缩进 4 + 短名 2 + 间隙 1 + 长名 9 + 间隙 2 + 标记 0 + 间隙 2。
const fixtureDescStart = 4 + 2 + 1 + 9 + 2 + 0 + 2

func TestHelpScreenStructure(t *testing.T) {
	screen := HelpScreen("PROG", "vTEST", 0, fixtureEntries()).Text()
	lines := strings.Split(screen, "\n")

	if lines[0] != "SSHFleet - 批量 SSH 运维工具（命令/脚本执行、文件上传下载）" {
		t.Errorf("首行不对：%q", lines[0])
	}
	if lines[1] != "版本: vTEST" {
		t.Errorf("版本行在首行下一行：%q", lines[1])
	}
	if !strings.Contains(screen, "  一组\n") {
		t.Errorf("组标题缩进 2 格：%q", screen)
	}
	if !strings.Contains(screen, "    -x --example  ") {
		t.Errorf("选项行缩进 4 格、短长名之间 1 空格、列间 2 空格：%q", screen)
	}
	if !strings.Contains(screen, "\n示例:\n  执行命令:   PROG -f nodes.csv -c \"ls -l\"\n") {
		t.Errorf("示例块标签列对齐：%q", screen)
	}
	if !strings.HasSuffix(screen, "\n更多用法、配置说明、示例：见 docs/\n") {
		t.Errorf("结尾一句：%q", screen)
	}
	if strings.Contains(screen, "\t") {
		t.Errorf("帮助里不出现制表符")
	}
}

func TestHelpScreenWrapsInOwnColumn(t *testing.T) {
	screen := HelpScreen("PROG", "vTEST", 80, fixtureEntries()).Text()
	for _, line := range strings.Split(screen, "\n") {
		if w := displayWidth(line); w > 80 {
			t.Errorf("宽度 80 下有行超宽（%d 列）：%q", w, line)
		}
	}
	// 说明列折出来的续行，前导空格数等于列起点
	lines := strings.Split(screen, "\n")
	for i, l := range lines {
		if !strings.HasPrefix(l, "    -x") {
			continue
		}
		if i+1 >= len(lines) || strings.TrimSpace(lines[i+1]) == "" {
			t.Fatalf("长说明在宽度 80 下应折出续行：\n%s", screen)
		}
		cont := lines[i+1]
		leading := len(cont) - len(strings.TrimLeft(cont, " "))
		if leading != fixtureDescStart {
			t.Errorf("续行起始列 want %d，got %d：%q", fixtureDescStart, leading, cont)
		}
		return
	}
	t.Fatalf("木偶数据的选项行不在屏里：\n%s", screen)
}

func TestHelpScreenFloorsDescriptionWidth(t *testing.T) {
	// 终端很窄：说明列仍按保底 24 列折，不挤成一列一个字（宁可整行超宽）
	screen := HelpScreen("PROG", "vTEST", 20, fixtureEntries()).Text()
	found := false
	for _, line := range strings.Split(screen, "\n") {
		if !strings.HasPrefix(line, "    -x") {
			continue
		}
		found = true
		body := strings.TrimPrefix(strings.TrimRight(line, " "), "-x --example  ")
		if displayWidth(body) < minDescWidth {
			t.Errorf("说明列保底 %d 列，实得 %d：%q", minDescWidth, displayWidth(body), line)
		}
	}
	if !found {
		t.Fatalf("木偶数据的选项行不在屏里：\n%s", screen)
	}
}

func TestCannotStartLine(t *testing.T) {
	got := CannotStartLine("加载失败", "原因那几行").Text()
	if got != "加载失败：原因那几行\n" {
		t.Errorf("起不来那句话的形态：%q", got)
	}
}
