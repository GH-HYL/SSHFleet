package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"sshfleet/internal/common"
	"sshfleet/internal/config"
)

// 帮助信息四列对齐（用户 2026-09-15 要求）：短选项 / 长选项 / 方括号小括号标记 / 说明，
// 各占一列；说明过长只在自己的列内折行，续行与首行说明同列起始，不串到其它列。

func helpTestCfg() *config.Config {
	cfg := &config.Config{}
	cfg.Execution.Sudo = true
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

// isTitleLine 组标题行：缩进与选项行不同（更浅），且不是选项。
func isTitleLine(ln string) bool {
	t := strings.TrimSpace(ln)
	if t == "" || strings.HasPrefix(t, "-") {
		return false
	}
	return leadingSpaces(ln) < 4
}

// leadingSpaces 行首空格数。
func leadingSpaces(ln string) int { return len(ln) - len(strings.TrimLeft(ln, " ")) }

// optionLinesAndCont 选项行 + 折行续行（去掉组标题与空行）。
func optionLinesAndCont(text string) []string {
	var out []string
	for _, ln := range optionsLines(text) {
		if strings.TrimSpace(ln) == "" || isTitleLine(ln) {
			continue
		}
		out = append(out, ln)
	}
	return out
}

// optionEntryLines 只取选项行（去掉组标题、空行与折行续行）。
func optionEntryLines(text string) []string {
	var out []string
	for _, ln := range optionLinesAndCont(text) {
		if strings.HasPrefix(strings.TrimSpace(ln), "-") {
			out = append(out, ln)
		}
	}
	return out
}

// helpTitleLines 取组标题行（既不空、也不是选项行、也不是折行续行）。
// 续行的缩进与选项行不同（落在说明列内，比组标题深），故复用 isTitleLine 的判据。
func helpTitleLines(text string) []string {
	var out []string
	for _, ln := range optionsLines(text) {
		if !isTitleLine(ln) {
			continue
		}
		out = append(out, strings.TrimSpace(ln))
	}
	return out
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
			block := optionLinesAndCont(usageText(cfg, "9.9.9", width))
			if len(block) == 0 {
				t.Fatal("选项块为空")
			}
			if width <= 0 {
				rows := 0
				for _, e := range entries {
					if !e.blank && e.group == "" {
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
				if e.blank || e.group != "" {
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

// 无短选项的条目：短选项列留白，长选项仍落在长选项列；组标题行缩进比选项行更浅。
func TestUsageTextEntriesWithoutShortOption(t *testing.T) {
	cfg := helpTestCfg()
	_, longCol, _, _ := helpColumns(cfg)
	text := usageText(cfg, "9.9.9", 120)
	found := false
	for _, ln := range optionEntryLines(text) {
		if displayColumnOf(ln, "--convert-secret") == longCol {
			found = true
			if strings.TrimSpace(prefixOf(ln, longCol)) != "" {
				t.Fatalf("无短选项时短选项列应留白：%q", ln)
			}
		}
	}
	if !found {
		t.Fatal("未找到 --convert-secret 行")
	}

	// 组标题用更浅的缩进，且不带任何列
	for _, ln := range optionsLines(text) {
		if !isTitleLine(ln) {
			continue
		}
		if got := leadingSpaces(ln); got != common.DisplayWidth(helpGroupIndent) {
			t.Fatalf("组标题应缩进 %d 列，实际 %d 列：%q", common.DisplayWidth(helpGroupIndent), got, ln)
		}
	}
}

// 长说明在窄终端下折行，续行仍落在说明列内，且折行不丢字。
func TestUsageTextWrapsLongDescriptionInOwnColumn(t *testing.T) {
	cfg := helpTestCfg()
	_, _, _, descCol := helpColumns(cfg)
	block := optionLinesAndCont(usageText(cfg, "9.9.9", 100))

	headIdx := -1
	for i, ln := range block {
		if strings.Contains(ln, "--convert-secret") {
			headIdx = i
			break
		}
	}
	if headIdx < 0 {
		t.Fatal("未找到 --convert-secret 行")
	}
	if headIdx+1 >= len(block) {
		t.Fatalf("100 列下长说明应折行，实际只有 %d 行：\n%s", len(block), strings.Join(block, "\n"))
	}

	// 该选项声明的完整描述（用于核对折行没丢字）
	var desc string
	for _, e := range helpEntries(cfg) {
		if e.long == "--convert-secret" {
			desc = e.desc
		}
	}
	if desc == "" {
		t.Fatal("未取到 --convert-secret 的描述")
	}

	// 收集它的说明行：首行 + 紧跟的续行（续行以说明列之前的空白开头）
	lines := []string{strings.TrimSpace(block[headIdx][len(prefixOf(block[headIdx], descCol)):])}
	for i := headIdx + 1; i < len(block); i++ {
		cont := block[i]
		if strings.TrimSpace(prefixOf(cont, descCol)) != "" {
			break
		}
		if strings.TrimSpace(cont) == "" {
			break
		}
		lines = append(lines, strings.TrimSpace(cont))
	}
	got := strings.ReplaceAll(strings.Join(lines, ""), " ", "")
	want := strings.ReplaceAll(desc, " ", "")
	if got != want {
		t.Fatalf("折行后说明内容不一致\n得到：%q\n期望：%q", got, want)
	}
}

// 选项表按用途分五组（每组一行组标题），用法拆成两行，末尾指向手册。
func TestUsageTextGroupsOptions(t *testing.T) {
	cfg := helpTestCfg()
	text := usageText(cfg, "9.9.9", 120)

	titles := helpTitleLines(text)
	want := []string{"模式（五选一）", "清单与目标路径", "执行选项", "密钥与凭据", "密钥管理（不执行批量任务）"}
	if len(titles) != len(want) {
		t.Fatalf("应有 %d 个组标题，实际 %d 个：%v", len(want), len(titles), titles)
	}
	for i, w := range want {
		if titles[i] != w {
			t.Fatalf("第 %d 个组标题应为 %q，实际 %q", i+1, w, titles[i])
		}
	}

	// 用法两行：批量执行一行、密钥管理一行。
	// 程序名与实现同源（args.go 取 filepath.Base(os.Args[0])），不写死平台后缀——
	// 测试二进制在 Windows 叫 cli.test.exe、Linux 叫 cli.test。
	progName := filepath.Base(os.Args[0])
	usageLines := 0
	for _, ln := range strings.Split(text, "\n") {
		if strings.HasPrefix(strings.TrimSpace(ln), progName) {
			usageLines++
		}
	}
	if usageLines != 2 {
		t.Fatalf("用法应为两行，实际 %d 行", usageLines)
	}

	// 模式那四行不再带标记列（标记只留给默认值那几行）
	for _, ln := range optionEntryLines(text) {
		for _, mode := range []string{"--command", "--script", "--upload", "--download"} {
			if strings.Contains(ln, mode) && strings.Contains(ln, "(命令模式)") {
				t.Fatalf("模式行不该再有标记列：%q", ln)
			}
		}
	}

	// 已删除的旧段落与旧选项名不应再出现
	for _, gone := range []string{"长选项与短选项等价", "上传并发说明", "建议并发数", "--disinteractive", "--nobash", "--no-bash", "-m --mode", "--file"} {
		if strings.Contains(text, gone) {
			t.Fatalf("帮助中不应再有 %q", gone)
		}
	}

	// 示例与手册指向
	for _, wantLine := range []string{"-c \"ls -l\"", "--yes", "-k", "见 docs/"} {
		if !strings.Contains(text, wantLine) {
			t.Fatalf("帮助缺少 %q：\n%s", wantLine, text)
		}
	}
}

// --sudo 的标记跟着配置里的值走（这是"不加参数时用的是哪种身份"）。
func TestUsageTextSudoTagFollowsConfig(t *testing.T) {
	on := helpTestCfg() // Sudo = true
	if text := usageText(on, "9.9.9", 120); !strings.Contains(text, "[当前配置: 开]") {
		t.Fatalf("配置为开时应标 [当前配置: 开]：\n%s", text)
	}
	off := helpTestCfg()
	off.Execution.Sudo = false
	text := usageText(off, "9.9.9", 120)
	if !strings.Contains(text, "[当前配置: 关]") {
		t.Fatalf("配置为关时应标 [当前配置: 关]：\n%s", text)
	}
	if strings.Contains(text, "配置是开时用它改回来") {
		t.Fatalf("配置为关时不该出现「配置是开时用它改回来」：\n%s", text)
	}
}

// 版本号行仍在首行下方，且含入口传入的版本串。
func TestUsageTextShowsVersion(t *testing.T) {
	text := usageText(helpTestCfg(), "1.2.3+abc1234", 100)
	if !strings.Contains(text, "版本: v1.2.3+abc1234") {
		t.Fatalf("帮助缺少版本行：\n%s", strings.SplitN(text, "\n", 4)[1])
	}
}

// Summary：解析结果按旧版 argparse.Namespace 的样子单行平铺。
//
// 旧版是 `tlog.success(f"参数解析成功,解析结果: {args}")`，{args} 走 argparse.Namespace
// 的 __repr__，输出形如：
//
//	Namespace(c='who -b', s='', u='', d='', f='nodes.csv', p='', sudo=False,
//	          t=None, T=None, n=None, r='v2_cmd', no_shell=False, yes=False, k=False)
//
// 这里逐字段对齐：字段名、空值写法（” 与 None）、布尔写法（True/False）都不能自作主张。
func TestSummaryMatchesArgparseNamespace(t *testing.T) {
	a := &Args{Command: "who -b", CsvFile: "nodes.csv", Remark: "v2_cmd"}
	got := a.Summary()
	want := "Namespace(c='who -b', s='', u='', d='', f='nodes.csv', p='', sudo=False, " +
		"t=None, T=None, n=None, r='v2_cmd', no_shell=False, yes=False, k=False, answer='', change_password='')"
	if got != want {
		t.Fatalf("解析结果格式不对\n实际：%s\n应为：%s", got, want)
	}
}

// -a 在日志里如实报出：给了就报值原文，没给是空串。
func TestSummaryAnswerField(t *testing.T) {
	a := &Args{Command: "pwd"}
	if got := a.Summary(); !strings.Contains(got, "answer=''") {
		t.Fatalf("没给 -a 时应报空串，实际：%s", got)
	}
	a = &Args{Command: "pwd", Answer: "1,请选择架构"}
	if got := a.Summary(); !strings.Contains(got, "answer='1,请选择架构'") {
		t.Fatalf("应报出 -a 的原始写法，实际：%s", got)
	}
}

// 指定了数值参数就报数值，未指定才是 None——不能因为「值为 0」就退化成 None。
func TestSummaryNumericFields(t *testing.T) {
	a := &Args{Command: "pwd", Number: 4, Timeout: 60, ConnectTimeout: 10}
	got := a.Summary()

	for _, want := range []string{"t=60", "T=10", "n=4"} {
		if !strings.Contains(got, want) {
			t.Fatalf("应有 %q，实际：%s", want, got)
		}
	}
	if strings.Contains(got, "=None") {
		t.Fatalf("三个数值都已指定，不该出现 None，实际：%s", got)
	}
}

// 超时未显式指定但已由程序补了默认值时，仍报出补后的值（日志要说明「这次实际用什么跑」）。
func TestSummaryTimeoutFilledByDefault(t *testing.T) {
	a := &Args{Command: "pwd", Timeout: 60, ConnectTimeout: 10} // timeoutRaw 为空 = 未显式指定
	got := a.Summary()
	if !strings.Contains(got, "t=60") || !strings.Contains(got, "T=10") {
		t.Fatalf("补过默认值就该报出来，实际：%s", got)
	}
}

// -k 只有两态，在 k= 里如实体现：没写 False，写了 True。
func TestSummaryKeyMode(t *testing.T) {
	cases := []struct {
		name string
		a    *Args
		want string
	}{
		{"未指定", &Args{Command: "pwd"}, "k=False"},
		{"写了 -k", &Args{Command: "pwd", Key: true}, "k=True"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.a.Summary(); !strings.Contains(got, c.want) {
				t.Fatalf("应有 %q，实际：%s", c.want, got)
			}
		})
	}
}

// 布尔字段用 Python 的 True / False 拼写。
func TestSummaryBooleanFields(t *testing.T) {
	a := &Args{Command: "pwd", NoShell: true, Disinteractive: true, Sudo: true, Key: true}
	got := a.Summary()
	for _, want := range []string{"no_shell=True", "yes=True", "sudo=True", "k=True"} {
		if !strings.Contains(got, want) {
			t.Fatalf("应有 %q，实际：%s", want, got)
		}
	}
	// 没给时是 False，不是空串、也不是 omits
	a = &Args{Command: "pwd"}
	got = a.Summary()
	for _, want := range []string{"no_shell=False", "yes=False", "sudo=False", "k=False"} {
		if !strings.Contains(got, want) {
			t.Fatalf("应有 %q，实际：%s", want, got)
		}
	}
}

// 内部状态字段不该泄漏到日志里——旧版 Namespace 里没有它们，读者也不需要。
func TestSummaryHidesInternalState(t *testing.T) {
	a := &Args{Command: "pwd", CsvFile: "n.csv"}
	got := a.Summary()
	for _, unwanted := range []string{
		"keyChanged", "timeoutInvalid", "numberRaw", "FIsInline",
		"timeoutRaw", "connectTimeoutRaw", "ModeName", "sudoFlag", "noSudoFlag",
	} {
		if strings.Contains(got, unwanted) {
			t.Fatalf("不该出现内部字段 %q，实际：%s", unwanted, got)
		}
	}
}

// 四种模式各自的字段都落在自己的位置上，不串位。
func TestSummaryByMode(t *testing.T) {
	cases := []struct {
		name string
		a    *Args
		want []string
	}{
		{"命令", &Args{Command: "pwd"}, []string{"c='pwd'", "s=''", "d=''"}},
		{"脚本", &Args{Script: "/x/t.sh"}, []string{"c=''", "s='/x/t.sh'"}},
		{"上传", &Args{Upload: "/x/a.txt", Path: "/opt/"}, []string{"u='/x/a.txt'", "p='/opt/'", "c=''"}},
		{"下载", &Args{Download: "/etc/x", Path: "D:/dl"}, []string{"d='/etc/x'", "p='D:/dl'", "c=''"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := c.a.Summary()
			for _, want := range c.want {
				if !strings.Contains(got, want) {
					t.Fatalf("应有 %q，实际：%s", want, got)
				}
			}
		})
	}
}

// --key-status 是旧版没有的字段，排在末尾补上；未指定时整个字段不出现。
func TestSummaryKeyStatus(t *testing.T) {
	a := &Args{KeyStatus: true}
	if got := a.Summary(); !strings.Contains(got, "key_status=True") {
		t.Fatalf("--key-status 应报 key_status=True，实际：%s", got)
	}
	a = &Args{Command: "pwd"}
	if got := a.Summary(); strings.Contains(got, "key_status") {
		t.Fatalf("未指定时不该出现 key_status，实际：%s", got)
	}
}

// 命令含单引号时不做转义处理——这个输出的用途是与旧日志逐字对照，加反斜杠反而对不上。
func TestSummaryKeepsRawValue(t *testing.T) {
	a := &Args{Command: "echo 'hi'"}
	if got := a.Summary(); !strings.Contains(got, `c='echo 'hi''`) {
		t.Fatalf("命令应原样打印，实际：%s", got)
	}
}

// 自动生成的备注不截断：它既进归档目录名，也**原样显示在确认屏上**——
// 屏上显示半截（如 afile.txt → afile.tx）使用者会以为那就是文件全名。
//
// 8 字符截断是 M1 骨架时加的，那时备注只喂 history 目录名；M2 把确认屏加进来、
// 把备注也显示了出来，却没人回头问「现在还要截吗」。目录名侧没有问题：
// 手工给的 -r 本就不截断，操作系统单段上限 255 字节远够用。
// 清洗（把 / : * ? 等换成 _）保留——那是目录名的实际约束，与长度无关。
func TestDefaultRemarkKeepsFullFirstWord(t *testing.T) {
	a := &Args{Command: "afile.txt --verbose"}
	got := defaultRemark(a)
	if got != "afile.txt" {
		t.Fatalf("自动备注应取首词全串，实际：%q", got)
	}
}

// 清洗仍然生效：路径分隔符照旧换成下划线（两边平台都要处理它）。
//
// 注意别把这条写成「所有特殊字符都换」——那样会把 Linux 上合法可用的字符也断言成
// 必须换掉。哪些字符该换由平台定，见下面那条。
func TestDefaultRemarkStillSanitizes(t *testing.T) {
	a := &Args{Command: `/usr/bin/env`}
	got := defaultRemark(a)
	if strings.Contains(got, "/") {
		t.Fatalf("路径分隔符 / 应被换成下划线，实际：%q", got)
	}
}

// 自动备注的字符处理按**运行平台**分：能用的直接用，不能用的才换下划线。
//
// 依据 File Name Portability（glibc 手册）：POSIX 下「任何字符（除 NUL）都可用于文件名，
// 且无长度限制」——所以 Linux 侧真正要处理的只有 `/`（它是路径分隔符，会把备注拆成多层
// 目录），其余（空格、冒号、星号…）一律原样留着。Windows 侧另有禁用集与保留设备名。
func TestDefaultRemarkKeepsCharsValidOnPlatform(t *testing.T) {
	a := &Args{Command: `deploy:prod*2`}
	got := defaultRemark(a)

	// `:` 与 `*` 在 POSIX 下是合法文件名字符，Windows 下不是。
	wantColonStar := runtime.GOOS != "windows"
	hasColon := strings.Contains(got, ":")
	hasStar := strings.Contains(got, "*")
	if hasColon != wantColonStar || hasStar != wantColonStar {
		t.Fatalf("平台 %s：冒号/星号保留应为 %v，实际备注：%q", runtime.GOOS, wantColonStar, got)
	}

	// `/` 两边都必须处理：它是路径分隔符，留着会把备注拆成多层目录。
	b := &Args{Command: "a/b"}
	if strings.Contains(defaultRemark(b), "/") {
		t.Fatalf("路径分隔符 / 必须换掉，实际：%q", defaultRemark(b))
	}
}

// 改密模式不传 -t 时取 timeout_execute 默认值：改密超时全靠 -t 兜底，缺了默认值
// ExecTimeout 就是 0，会话刚建立即被本地计时掐断（2026-09-29 实测）。显式指定不覆盖。
func TestParsePasswdModeDefaultsTimeout(t *testing.T) {
	cfg := helpTestCfg()
	args := []string{"-f", "172.28.118.49,22,root,pw", "--change-password", "N3wpw#2026"}
	a, err := Parse(cfg, "test", args)
	if err != nil {
		t.Fatalf("Parse 报错：%v", err)
	}
	if a.Timeout != cfg.Execution.TimeoutExecute {
		t.Fatalf("改密模式应取 timeout_execute 默认值：%d，实际：%d", cfg.Execution.TimeoutExecute, a.Timeout)
	}
	a, err = Parse(cfg, "test", append(args, "-t", "5"))
	if err != nil {
		t.Fatalf("Parse 报错：%v", err)
	}
	if a.Timeout != 5 {
		t.Fatalf("显式 -t 不该被默认值覆盖：应为 5，实际：%d", a.Timeout)
	}
}

// 远端路径里的空格**不再拦**：Linux 的文件名允许空格与括号，拦它等于替使用者
// 判他的目标路径（ADR-0025 更新记录 2026-10-26）。值原样交给远端，远端不认它自己会报。
func TestRemotePathSpaceIsPassedThrough(t *testing.T) {
	dir := t.TempDir()
	local := filepath.Join(dir, "f.txt")
	if err := os.WriteFile(local, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name  string
		args  []string
		field func(*Args) string
		want  string
	}{
		{"-d 中间带空格", []string{"-d", "/var/my log", "-p", dir}, func(a *Args) string { return a.Download }, "/var/my log"},
		{"上传 -p 中间带空格", []string{"-u", local, "-p", "/opt/my dir/"}, func(a *Args) string { return a.Path }, "/opt/my dir/"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			args := append([]string{"-f", "172.28.118.49,22,root,pw"}, c.args...)
			a, err := Parse(helpTestCfg(), "test", args)
			if err != nil {
				t.Fatalf("Parse 不该报错：%v", err)
			}
			if err := CheckArguments(a); err != nil {
				t.Fatalf("空格不该被拦：%v", err)
			}
			if got := c.field(a); got != c.want {
				t.Fatalf("值应逐字保留：期望 %q，实际 %q", c.want, got)
			}
		})
	}
}

// 两端空白过滤掉：那是复制粘贴带进来的，不是路径的一部分，也不该成为一条错误。
func TestRemotePathOuterSpaceIsTrimmed(t *testing.T) {
	dir := t.TempDir()
	a, perr := Parse(helpTestCfg(), "test", []string{"-f", "172.28.118.49,22,root,pw", "-d", " /var/log ", "-p", dir})
	if perr != nil {
		t.Fatalf("Parse 不该报错：%v", perr)
	}
	if err := CheckArguments(a); err != nil {
		t.Fatalf("两端空白应被过滤掉、不该报错：%v", err)
	}
	if a.Download != "/var/log" {
		t.Fatalf("两端空白应去掉，实际 %q", a.Download)
	}
}

// 值被 shell 按空格切开时，报错要点出是**哪个选项**的值断在这里（工单 09）。
// 这一句必须早于任何 `os` 调用：`os` 只拿到前半段，它只会说「不存在」。
func TestExtraArgsAttributesSplitValue(t *testing.T) {
	dir := t.TempDir()
	cases := []struct {
		name  string
		args  []string
		owner string
	}{
		{"-p 的值被空格切开", []string{"-u", "afile.txt", "-p", "/opt/my", "dir/"}, "-p"},
		{"-d 的值被空格切开", []string{"-d", "/var/my", "log", "-p", dir}, "-d"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			args := append([]string{"-f", "172.28.118.49,22,root,pw"}, c.args...)
			_, err := Parse(helpTestCfg(), "test", args)
			if err == nil {
				t.Fatal("多出参数应报错")
			}
			msg := err.Error()
			if !strings.Contains(msg, c.owner) || !strings.Contains(msg, "没法识别") {
				t.Fatalf("应点出 %s 且说明多出一段，实际：%v", c.owner, err)
			}
			if !strings.Contains(msg, "双引号") {
				t.Fatalf("应给双引号写法，实际：%v", err)
			}
		})
	}
}

// 回溯不出归属时照样报，只是不点名选项——不能因为说不清就放过。
func TestExtraArgsWithoutOwnerStillReports(t *testing.T) {
	_, err := Parse(helpTestCfg(), "test", []string{"stray"})
	if err == nil || !strings.Contains(err.Error(), "出现多余的参数") {
		t.Fatalf("应报多余参数，实际：%v", err)
	}
}

// 内容有问题的报错（二进制 / 非 UTF-8 / 清单不是文本）必须点名是哪个参数带进来的。
// 此前这几条只甩一个文件名，同一个「%s 是二进制文件」在 -s、-f、-a 三处复用、
// 只有提示段不同，使用者看不出是谁出的事（2026-10-15 审计实测）。
func TestCheckErrorsNameTheirFlag(t *testing.T) {
	dir := t.TempDir()

	bin := filepath.Join(dir, "b.bin")
	if err := os.WriteFile(bin, []byte{0x00, 0x01, 0x02}, 0o644); err != nil {
		t.Fatal(err)
	}
	notUTF8 := filepath.Join(dir, "u.sh")
	if err := os.WriteFile(notUTF8, []byte{0xff, 0xfe, 'x'}, 0o644); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name string
		args []string
		flag string
	}{
		{"-s 是二进制", []string{"-f", "172.28.118.49,22,root,pw", "-s", bin}, "-s"},
		{"-s 非 UTF-8", []string{"-f", "172.28.118.49,22,root,pw", "-s", notUTF8}, "-s"},
		{"-f 是二进制", []string{"-f", bin, "-c", "uptime"}, "-f"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a, err := Parse(helpTestCfg(), "test", c.args)
			if err != nil {
				t.Fatalf("Parse 不该在这里报错：%v", err)
			}
			err = CheckArguments(a)
			if err == nil {
				t.Fatal("应报错，实际通过")
			}
			if !strings.Contains(err.Error(), c.flag) {
				t.Fatalf("报错应点名 %s，实际：%v", c.flag, err)
			}
		})
	}
}

// 命令行给的路径一个字节都不改：去空白、换分隔符、压 `..`、补结尾 `/` 这些加工全部退出
// 解析阶段（ADR-0025 决定 1）。以前 `-s a\b.sh` 会被悄悄变成 `a/b.sh`，打开的不是写的那条路径。
func TestParseKeepsPathValuesVerbatim(t *testing.T) {
	cases := []struct {
		name string
		args []string
		got  func(*Args) string
		want string
	}{
		{"-s 反斜杠", []string{"-s", `docs/a\b.sh`, "-f", "172.28.118.49,22,root,pw"}, func(a *Args) string { return a.Script }, `docs/a\b.sh`},
		{"-s 两端空白", []string{"-s", " a.sh ", "-f", "172.28.118.49,22,root,pw"}, func(a *Args) string { return a.Script }, " a.sh "},
		{"-u 重复分隔符与 ..", []string{"-u", "a/../b//c/", "-p", "/opt/", "-f", "172.28.118.49,22,root,pw"}, func(a *Args) string { return a.Upload }, "a/../b//c/"},
		{"上传 -p 反斜杠", []string{"-u", "afile.txt", "-p", `\opt\app\`, "-f", "172.28.118.49,22,root,pw"}, func(a *Args) string { return a.Path }, `\opt\app\`},
		{"-d 反斜杠", []string{"-d", `\var\log`, "-p", ".", "-f", "172.28.118.49,22,root,pw"}, func(a *Args) string { return a.Download }, `\var\log`},
		{"-d 里的 ..", []string{"-d", "/var/log/../log", "-p", ".", "-f", "172.28.118.49,22,root,pw"}, func(a *Args) string { return a.Download }, "/var/log/../log"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a, err := Parse(helpTestCfg(), "test", c.args)
			if err != nil {
				t.Fatalf("Parse 不该报错：%v", err)
			}
			if got := c.got(a); got != c.want {
				t.Fatalf("值应逐字相同：期望 %q，实际 %q", c.want, got)
			}
		})
	}
}

// 上传的 -p 不以 / 开头时报错，回显的必须是**原值**（不是被改写后的形态）——
// 这条是「程序不再替他换分隔符」最直接的证据。
func TestUploadPathErrorEchoesRawValue(t *testing.T) {
	a, err := Parse(helpTestCfg(), "test", []string{"-u", "afile.txt", "-p", `\opt\app\`, "-f", "172.28.118.49,22,root,pw"})
	if err != nil {
		t.Fatalf("Parse 不该报错：%v", err)
	}
	cerr := CheckArguments(a)
	if cerr == nil {
		t.Fatal(`-p '\\opt\\app\\' 不以 / 开头，应报错`)
	}
	if !strings.Contains(cerr.Error(), `\opt\app\`) {
		t.Fatalf("报错里应回显原值 %q，实际：%v", `\opt\app\`, cerr)
	}
}

// 两端空白过滤发生在解析阶段，**早于取备注**：否则会出现 `d` 已去空白、
// `r` 还带着多打那个空格的两种形态（真机跑 ` -d " /tmp/app.log " ` 撞到过）。
func TestRemotePathOuterSpaceIsTrimmedBeforeRemark(t *testing.T) {
	dir := t.TempDir()
	a, perr := Parse(helpTestCfg(), "test", []string{"-f", "172.28.118.49,22,root,pw", "-d", " /tmp/app.log ", "-p", dir})
	if perr != nil {
		t.Fatalf("Parse 不该报错：%v", perr)
	}
	if a.Download != "/tmp/app.log" {
		t.Fatalf("d 应去两端空白，实际 %q", a.Download)
	}
	if a.Remark != "app.log" {
		t.Fatalf("备注应按去空白后的值取名，实际 %q", a.Remark)
	}
}

// 远端值里的回车换行仍要拦：那不是「路径里的一个字符」，是参数被换行打断的现场。
// 交出去远端只会回一句看不懂的失败，本机这一句还指得准。
func TestRemotePathRejectsControlChars(t *testing.T) {
	dir := t.TempDir()
	cases := []struct {
		name string
		args []string
		want string
	}{
		{"-d 中间有换行", []string{"-d", "/var/log\napp", "-p", dir}, "回车或换行"},
		{"上传 -p 中间有回车", []string{"-u", "x.txt", "-p", "/opt/app/\r/"}, "回车或换行"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			args := append([]string{"-f", "172.28.118.49,22,root,pw"}, c.args...)
			a, perr := Parse(helpTestCfg(), "test", args)
			if perr != nil {
				t.Fatalf("Parse 不该报错：%v", perr)
			}
			cerr := CheckArguments(a)
			if cerr == nil || !strings.Contains(cerr.Error(), c.want) {
				t.Fatalf("应报 %s，实际：%v", c.want, cerr)
			}
		})
	}
}

// 解析结果里那一格写的是**这次要设的新密码**的脱敏形态，与加密开关无关。
//
// 加密模式下 --change-password 收的是密文文件路径，真正的密码解出来落在 NewPassword。
// 日志这行是事后排查「这次到底按什么参数跑的」唯一的凭据，写一串路径的脱敏等于什么都没写。
func TestSummaryPasswdFieldShowsRealNewPassword(t *testing.T) {
	cases := []struct {
		name string
		a    *Args
	}{
		{
			name: "加密模式：旗标值是密文文件路径",
			a:    &Args{ChangePasswordRaw: "/home/hyl/secret/new_pw.txt", NewPassword: "NewPass#2026"},
		},
		{
			name: "明文模式：旗标值就是新密码",
			a:    &Args{ChangePasswordRaw: "NewPass#2026", NewPassword: "NewPass#2026"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := c.a.Summary()
			if !strings.Contains(got, "change_password='Ne****26'") {
				t.Fatalf("change_password 应为新密码的脱敏形态 Ne****26，实际：%s", got)
			}
		})
	}
}

// 防御性分支：给了 --change-password 却还没解析新密码时，日志那格写「未解析」。
//
// 生产路径走不到（Summary 的唯一调用点在合规检查之后）。这条测试只钉住「万一走到时
// 不拿 **** 顶替」——**** 与「4 位以内的密码」同形，看日志的人分不出来。
func TestSummaryPasswdFieldBeforeDecode(t *testing.T) {
	a := &Args{ChangePasswordRaw: "/home/hyl/secret/new_pw.txt"} // 未经 CheckArguments
	if got := a.Summary(); !strings.Contains(got, "change_password='（未解析）'") {
		t.Fatalf("未解析时应写明，实际：%s", got)
	}
}

// 远端路径必须是绝对路径：远端拿相对路径当「登录目录下的那个」，落点全凭运气。
// 这是本机对远端值唯一还判的一条（另加「没有回车换行」），其余形态一概不判。
func TestRemotePathMustBeAbsolute(t *testing.T) {
	dir := t.TempDir()
	local := filepath.Join(dir, "f.txt")
	if err := os.WriteFile(local, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name string
		args []string
		want string
	}{
		{"-d 相对路径", []string{"-d", "var/log", "-p", dir}, "必须是绝对路径"},
		{"上传 -p 相对路径", []string{"-u", local, "-p", "opt/app/"}, "必须是绝对路径"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			args := append([]string{"-f", "172.28.118.49,22,root,pw"}, c.args...)
			a, err := Parse(helpTestCfg(), "test", args)
			if err != nil {
				t.Fatalf("Parse 不该在这里报错：%v", err)
			}
			cerr := CheckArguments(a)
			if cerr == nil {
				t.Fatalf("%s 应被拦下，实际通过", c.name)
			}
			if !strings.Contains(cerr.Error(), c.want) {
				t.Fatalf("报错应含 %q，实际：%v", c.want, cerr)
			}
		})
	}
}

// [locale] english 从配置进 Args 的这一跳。全仓只有 args.go 一处把配置写进这个字段，
// 它错了开关在任何形态下都不生效，而下发行照样是合法串——不会有别的东西报警。
func TestParseCarriesLocaleEnglish(t *testing.T) {
	for _, want := range []bool{false, true} {
		cfg := helpTestCfg()
		cfg.Locale.English = want
		a, err := Parse(cfg, "test", []string{"-c", "who -b"})
		if err != nil {
			t.Fatalf("Parse 报错：%v", err)
		}
		if a.English != want {
			t.Fatalf("配置 english=%v 应原样进 Args，实际 %v", want, a.English)
		}
	}
}
