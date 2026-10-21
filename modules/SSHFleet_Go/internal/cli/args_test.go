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

// 远端路径带空格：文案必须点名**当前这个选项**。
// 此前 pathWithoutSpace 被多个选项共用，文案里没有选项名、举例写死 -u，
// 于是别的选项出问题也提示 -u，使用者对着提示改不动自己的命令行（2026-10-15 审计实测）。
//
// 2026-10-15 口径收窄后，本检查只作用于两个远端参数（-u 的 -p、-d）——
// 本地四个能验存在性，不走这条。
func TestParsePathSpaceErrorNamesCurrentFlag(t *testing.T) {
	cfg := helpTestCfg()
	cases := []struct {
		flag string
		args []string
	}{
		{"-d", []string{"-f", "172.28.118.49,22,root,pw", "-d", "/var/my log", "-p", "./logs/"}},
		{"-p", []string{"-f", "172.28.118.49,22,root,pw", "-u", "afile.txt", "-p", "/opt/my dir/"}},
	}
	for _, c := range cases {
		_, err := Parse(cfg, "test", c.args)
		if err == nil {
			t.Fatalf("%s 带空格应报错，实际通过", c.flag)
		}
		if !strings.Contains(err.Error(), c.flag) {
			t.Fatalf("%s 带空格的报错应点名 %s，实际：%v", c.flag, c.flag, err)
		}
	}
}

// 远端路径带空格的提示里不能出现**别的**选项。
func TestParsePathSpaceErrorHasNoWrongFlag(t *testing.T) {
	cfg := helpTestCfg()
	_, err := Parse(cfg, "test", []string{"-f", "172.28.118.49,22,root,pw", "-d", "/var/my log", "-p", "./logs/"})
	if err == nil {
		t.Fatal("-d 带空格应报错，实际通过")
	}
	msg := err.Error()
	for _, other := range []string{"-s", "-u", "-f"} {
		if strings.Contains(msg, other) {
			t.Fatalf("-d 的报错不该出现别的选项 %s，实际：%v", other, err)
		}
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

// 空格检查只该管**远端**参数——远端验不了存在性，只能靠匹配拦。
// 本地四个（-s / -u / -f / -d 的 -p）能验存在性，报「不存在」比报「有空格」准得多，
// 且空格检查会把更准的报错截胡（2026-10-15 审计实测：七条真错被截）。
func TestPathSpaceCheckSkipsLocalPaths(t *testing.T) {
	cfg := helpTestCfg()

	local := []struct {
		name string
		args []string
	}{
		{"-s 带空格的命令", []string{"-f", "172.28.118.49,22,root,pw", "-s", "cat /etc/os-release"}},
		{"-u 带空格的值", []string{"-f", "172.28.118.49,22,root,pw", "-u", "my file.txt", "-p", "/opt/"}},
		{"-f 带空格的路径", []string{"-f", "my list.csv", "-c", "uptime"}},
		{"-d 的本地 -p 带空格", []string{"-f", "172.28.118.49,22,root,pw", "-d", "/var/log", "-p", "./my dir/"}},
	}
	for _, c := range local {
		t.Run(c.name, func(t *testing.T) {
			a, err := Parse(cfg, "test", c.args)
			if err != nil {
				// 解析阶段就不该拿空格拦本地路径
				if strings.Contains(err.Error(), "不能有空格") {
					t.Fatalf("本地路径不该被空格检查拦下：%v", err)
				}
				return // 别的原因报错不管——本条只测空格检查
			}
			if err := CheckArguments(a); err != nil && strings.Contains(err.Error(), "不能有空格") {
				t.Fatalf("本地路径不该被空格检查拦下：%v", err)
			}
		})
	}
}

// 远端两个参数必须保留空格检查：它们验不了存在性，「路径带空格」是唯一能拦的信号。
func TestPathSpaceCheckKeepsRemotePaths(t *testing.T) {
	cfg := helpTestCfg()

	remote := []struct {
		name string
		args []string
	}{
		{"-u 的远端 -p 带空格", []string{"-f", "172.28.118.49,22,root,pw", "-u", "afile.txt", "-p", "/opt/my dir/"}},
		{"-d 的远端值带空格", []string{"-f", "172.28.118.49,22,root,pw", "-d", "/var/my log", "-p", "./logs/"}},
	}
	for _, c := range remote {
		t.Run(c.name, func(t *testing.T) {
			a, err := Parse(cfg, "test", c.args)
			if err == nil {
				err = CheckArguments(a)
			}
			if err == nil {
				t.Fatal("远端路径带空格应报错，实际通过")
			}
			if !strings.Contains(err.Error(), "不能有空格") {
				t.Fatalf("远端路径带空格应报空格，实际：%v", err)
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
			a:    &Args{ChangePassword: "/home/hyl/secret/new_pw.txt", NewPassword: "NewPass#2026"},
		},
		{
			name: "明文模式：旗标值就是新密码",
			a:    &Args{ChangePassword: "NewPass#2026", NewPassword: "NewPass#2026"},
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
	a := &Args{ChangePassword: "/home/hyl/secret/new_pw.txt"} // 未经 CheckArguments
	if got := a.Summary(); !strings.Contains(got, "change_password='（未解析）'") {
		t.Fatalf("未解析时应写明，实际：%s", got)
	}
}
