package cli

import (
	"fmt"
	"strings"
	"testing"

	"sshfleet/internal/common"
	"sshfleet/internal/config"
)

// 帮助信息四列对齐（用户 2026-09-15 要求）：短选项 / 长选项 / 方括号小括号标记 / 说明，
// 各占一列；说明过长只在自己的列内折行，续行与首行说明同列起始，不串到其它列。

func helpTestCfg() *config.Config {
	cfg := &config.Config{}
	cfg.Execution.Mode = "sudo"
	cfg.Execution.TimeoutExecute = 60
	cfg.Execution.TimeoutTransfer = 300
	cfg.Execution.TimeoutConnect = 10
	return cfg
}

// optionsLines 取「选项:」与「示例:」之间的原始行（含分组空行）。
func optionsLines(text string) []string {
	var lines []string
	in := false
	for _, ln := range strings.Split(text, "\n") {
		if strings.TrimSpace(ln) == "选项:" {
			in = true
			continue
		}
		if !in {
			continue
		}
		if strings.TrimSpace(ln) == "示例:" {
			break
		}
		lines = append(lines, ln)
	}
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

// optionsBlock 只取非空的选项行（分组空行由 optionsLines 负责）。
func optionsBlock(text string) []string {
	var block []string
	for _, ln := range optionsLines(text) {
		if strings.TrimSpace(ln) != "" {
			block = append(block, ln)
		}
	}
	return block
}

// optionGroupCount 分组数 = 空行数 + 1。
func optionGroupCount(text string) int {
	blankRuns, prevBlank, hasOption := 0, true, false
	for _, ln := range optionsLines(text) {
		blank := strings.TrimSpace(ln) == ""
		if !blank {
			hasOption = true
		}
		if blank && !prevBlank {
			blankRuns++
		}
		prevBlank = blank
	}
	if !hasOption {
		return 0
	}
	return blankRuns + 1 // 分组数 = 组间空行数 + 1
}

// helpColumns 四列的起始显示列（与渲染实现同源口径：缩进 2 + 各列宽 + 列间 2 空格）。
func helpColumns(cfg *config.Config) (shortCol, longCol, tagCol, descCol int) {
	entries := helpEntries(cfg)
	wShort, wLong, wTag := 0, 0, 0
	for _, e := range entries {
		wShort = max(wShort, common.DisplayWidth(e.short))
		wLong = max(wLong, common.DisplayWidth(e.long))
		wTag = max(wTag, common.DisplayWidth(e.tag))
	}
	shortCol = common.DisplayWidth(helpIndent)
	longCol = shortCol + wShort + common.DisplayWidth(helpOptGap)
	tagCol = longCol + wLong + common.DisplayWidth(helpGap)
	descCol = tagCol + wTag + common.DisplayWidth(helpGap)
	return
}

// displayColumnOf 子串首次出现处的显示列（-1 表示没出现）。
func displayColumnOf(s, sub string) int {
	i := strings.Index(s, sub)
	if i < 0 {
		return -1
	}
	return common.DisplayWidth(s[:i])
}

// prefixOf 取前 col 个显示列对应的前缀。
func prefixOf(s string, col int) string {
	w := 0
	for i, r := range s {
		if w >= col {
			return s[:i]
		}
		w += common.RuneWidth(r)
	}
	return s
}

func TestUsageTextFourColumnsAligned(t *testing.T) {
	cfg := helpTestCfg()
	entries := helpEntries(cfg)
	shortCol, longCol, tagCol, descCol := helpColumns(cfg)

	for _, width := range []int{0, 60, 72, 80, 100, 140, 240} {
		width := width
		t.Run(fmt.Sprintf("width_%d", width), func(t *testing.T) {
			block := optionsBlock(usageText(cfg, "9.9.9", width))
			if len(block) == 0 {
				t.Fatal("选项块为空")
			}
			if width <= 0 {
				rows := 0
				for _, e := range entries {
					if !e.blank {
						rows++
					}
				}
				if len(block) != rows {
					t.Fatalf("不折行时应有 %d 行选项，实际 %d 行", rows, len(block))
				}
			}

			// 允许的最大行宽：终端宽度，但说明列有保底宽度（太窄时宁可超宽）
			maxLine := 0
			if width > 0 {
				maxLine = max(width, descCol+minDescWidth)
			}
			// ① 每行的「表头区」恰好占 descCol 列，说明列从正文开始；整行不超允许宽度
			for i, ln := range block {
				prefix := prefixOf(ln, descCol)
				if got := common.DisplayWidth(prefix); got != descCol {
					t.Fatalf("第 %d 行表头区应为 %d 列，实际 %d 列：%q", i+1, descCol, got, ln)
				}
				rest := ln[len(prefix):]
				if rest == "" || strings.HasPrefix(rest, " ") {
					t.Fatalf("第 %d 行说明列起始处应是正文：%q", i+1, ln)
				}
				if maxLine > 0 {
					if w := common.DisplayWidth(ln); w > maxLine {
						t.Fatalf("第 %d 行 %d 列超宽（上限 %d）：%q", i+1, w, maxLine, ln)
					}
				}
			}

			// ② 每个选项都落在自己那几列上
			for _, e := range entries {
				if e.blank {
					continue
				}
				var head string
				for _, ln := range block {
					if displayColumnOf(ln, e.long) == longCol {
						head = ln
						break
					}
				}
				if head == "" {
					t.Fatalf("未找到 %s 的选项行\n%s", e.long, strings.Join(block, "\n"))
				}
				if e.short != "" {
					if got := displayColumnOf(head, e.short); got != shortCol {
						t.Fatalf("短选项 %s 应在第 %d 列，实际第 %d 列：%q", e.short, shortCol, got, head)
					}
				}
				if e.tag != "" {
					if got := displayColumnOf(head, e.tag); got != tagCol {
						t.Fatalf("标记 %s 应在第 %d 列，实际第 %d 列：%q", e.tag, tagCol, got, head)
					}
				}
			}
		})
	}
}

// 无短选项的条目：短选项列留白，长选项仍落在长选项列。
func TestUsageTextEntriesWithoutShortOption(t *testing.T) {
	cfg := helpTestCfg()
	_, longCol, _, _ := helpColumns(cfg)
	found := false
	for _, ln := range optionsBlock(usageText(cfg, "9.9.9", 120)) {
		if displayColumnOf(ln, "--convert-password") == longCol {
			found = true
			if strings.TrimSpace(prefixOf(ln, longCol)) != "" {
				t.Fatalf("无短选项时短选项列应留白：%q", ln)
			}
		}
	}
	if !found {
		t.Fatal("未找到 --convert-password 行")
	}
}

// 长说明在窄终端下折行，续行仍落在说明列内。
func TestUsageTextWrapsLongDescriptionInOwnColumn(t *testing.T) {
	cfg := helpTestCfg()
	_, _, _, descCol := helpColumns(cfg)
	block := optionsBlock(usageText(cfg, "9.9.9", 100))

	headIdx := -1
	for i, ln := range block {
		if strings.Contains(ln, "--convert-password") {
			headIdx = i
			break
		}
	}
	if headIdx < 0 {
		t.Fatal("未找到 --convert-password 行")
	}
	if headIdx+1 >= len(block) {
		t.Fatalf("100 列下长说明应折行，实际只有 %d 行：%v", len(block), block)
	}
	cont := block[headIdx+1]
	if strings.TrimSpace(cont) == "" {
		t.Fatalf("续行不应为空：%q", cont)
	}
	if strings.TrimSpace(prefixOf(cont, descCol)) != "" {
		t.Fatalf("续行在说明列之前应全为空格：%q", cont)
	}
	if !strings.Contains(cont, "凭据") && !strings.Contains(cont, "格式") {
		t.Fatalf("续行内容异常：%q", cont)
	}
}

// 选项表按用途分组（组间空行），且用法只保留一行。
func TestUsageTextGroupsOptions(t *testing.T) {
	cfg := helpTestCfg()
	text := usageText(cfg, "9.9.9", 120)
	if got := optionGroupCount(text); got != 4 {
		t.Fatalf("选项应分 4 组，实际 %d 组：\n%s", got, text)
	}
	// 用法只有一行（工具选项不再单列一行）
	usageLines := 0
	for _, ln := range strings.Split(text, "\n") {
		if strings.HasPrefix(strings.TrimSpace(ln), "cli.test.exe") {
			usageLines++
		}
	}
	if usageLines != 1 {
		t.Fatalf("用法应只有一行，实际 %d 行", usageLines)
	}
	// 已删除的旧段落不应再出现
	for _, gone := range []string{"长选项与短选项等价", "上传并发说明", "建议并发数"} {
		if strings.Contains(text, gone) {
			t.Fatalf("帮助中不应再有 %q", gone)
		}
	}
	// 示例含生成与转换两条
	for _, want := range []string{"--gen-key", "--convert-password ~/.MyPW/pw.txt"} {
		if !strings.Contains(text, want) {
			t.Fatalf("示例缺少 %q", want)
		}
	}
}

// 版本号行仍在首行下方，且含入口传入的版本串。
func TestUsageTextShowsVersion(t *testing.T) {
	text := usageText(helpTestCfg(), "1.2.3+abc1234", 100)
	if !strings.Contains(text, "版本: v1.2.3+abc1234") {
		t.Fatalf("帮助缺少版本行：\n%s", strings.SplitN(text, "\n", 4)[1])
	}
}
