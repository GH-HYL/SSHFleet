package message

// 两处产出：屏幕上的统计块，与归档里的 `report.txt`。
//
// 都住这里，因为两样都是「给人看的字」：分类名、提示语、退出码含义、栏目名，一处成形。
// 报告站只负责「解运行上下文、遮秘密、写盘、上屏」，一个字的版面都不自己排。
//
// 版式逐字照老版 `output/terminal.go` 的统计块与 `output/report.go` 整份。两处有一处
// 差别是刻意的：屏幕写「失败分类统计 >>>」，报告写「失败分类统计 -→」——老版就不同，
// 不是笔误。

import (
	"fmt"
	"strconv"
	"strings"

	"sshfleet2/internal/contract"
)

// 跑没跑齐那两个结论字。结算站算出来、报告站照着分岔，字只在这里。
const (
	VerifyPassed = "通过"
	VerifyFailed = "异常"
)

const (
	// statsBarWidth 统计块上下那两条线的宽度：老版就是 60 个「═」。
	statsBarWidth = 60
	// reportHeadline 是 `report.txt` 顶上那一行，两侧各 29 个等号。
	reportHeadline = "=============================执行结果统计报告============================="
	// reportTimeLayout 报告里两个时间戳的写法：到微秒。
	reportTimeLayout = "2006-01-02 15:04:05.000000"
)

// exitCodeHints 常见退出码含义（Unix 通用语义，逐字照老版 EXIT_CODE_HINTS）。
//
// 关掉「提示」开关时屏幕上那一行、以及退出码分类的提示语，都从这里取。
var exitCodeHints = map[int]string{
	1:   "一般性错误",
	2:   "命令用法错误",
	126: "命令不可执行（权限不足）",
	127: "命令未找到",
	130: "被中断（SIGINT / Ctrl+C）",
	137: "被强制杀死（SIGKILL）",
	143: "被终止（SIGTERM）",
	255: "命令执行失败",
}

// exitCodeFailTip 「执行失败(退出码N)」这条分类的固定说明：它的名字里带数字，判据文件
// 按名字查不到，只能由工具按退出码补上含义。
const exitCodeFailTip = "命令自身失败，或命令没跑起来被拒（未识别出具体原因）"

// StatisticsLine 是屏幕上那段统计块（带色码，末尾有换行）。
//
// showTips 是配置 `enable.show_category_tips` 的原值，keywordsPath 是判据文件的展示
// 路径——两样都从运行上下文里解出来，报告站手上没有配置。
func StatisticsLine(l contract.Ledger, showTips bool, keywordsPath string) Line {
	var b strings.Builder
	bar := strings.Repeat("═", statsBarWidth)

	b.WriteString(bar + "\n")
	fmt.Fprintf(&b, "  总耗时：%.2f 秒\n", l.CostTime)

	switch {
	case l.Verify == VerifyPassed:
		fmt.Fprintf(&b, "  %s节点总数：%s %d  %s完成总数：%s%d\n",
			ansiCyan, ansiReset, l.Total, ansiCyan, ansiReset, l.Done)
	case l.Interrupted:
		// 中断：另起一栏，把「未执行」单独列出——使用者自己按的中断，不与「跑错了」共用「异常」。
		fmt.Fprintf(&b, "  %s节点总数：%s %d  %s完成总数：%s%d  %s未执行：%s%d\n",
			ansiCyan, ansiReset, l.Total, ansiCyan, ansiReset, l.Done,
			ansiCyan, ansiReset, l.NotRun)
	default:
		fmt.Fprintf(&b, "  %s节点总数：%s %d  %s完成总数：%s%d  %s总数校验：%s%s%s%s\n",
			ansiCyan, ansiReset, l.Total, ansiCyan, ansiReset, l.Done,
			ansiCyan, ansiReset, ansiRed, l.Verify, ansiReset)
	}

	if l.FailCounts > 0 {
		fmt.Fprintf(&b, "  %s成功：%s %d   %s失败：%s %d\n",
			ansiGreen, ansiReset, l.SuccessCounts, ansiRed, ansiReset, l.FailCounts)
	} else {
		fmt.Fprintf(&b, "  %s成功：%s %d\n", ansiGreen, ansiReset, l.SuccessCounts)
	}

	if len(l.FailCategories) > 0 {
		var known, fallback []string
		for _, c := range l.FailCategories {
			// 「分类名 ×3」：× 紧贴数字，一眼看出是"个数"——免得与分类名里嵌的数字
			// （「执行失败(退出码1)」）混起来。
			item := fmt.Sprintf("%s%s ×%s%d", ansiYellow, c.Category, ansiReset, c.Count)
			if isFallbackCategory(l.Tables, c.Category) {
				fallback = append(fallback, item)
			} else {
				known = append(known, item)
			}
		}
		if len(known) > 0 {
			fmt.Fprintf(&b, "  %s失败分类统计%s >>>  %s\n", ansiRed, ansiReset, strings.Join(known, "  "))
		} else {
			fmt.Fprintf(&b, "  %s失败分类统计%s >>>\n", ansiRed, ansiReset)
		}
		for _, item := range fallback {
			fmt.Fprintf(&b, "    %s\n", item)
		}
		if len(fallback) > 0 {
			// 未归类的那几类显示的是报错原文；末尾给一条「怎么把它们归到类里」的引导——
			// 使用者反复看到同一个原因却没人告诉他能否自己加分类，是这份统计最大的浪费。
			fmt.Fprintf(&b, "  %s未归类 %d 类：%s上方直接显示的是报错原文，没能匹配到已有分类。\n",
				ansiYellow, len(fallback), ansiReset)
			fmt.Fprintf(&b, "  若其中某类会反复出现，可把它的特征词补进 %s，\n", keywordsPath)
			fmt.Fprintf(&b, "  之后就会单独归为一类。\n")
		}
		if showTips {
			if lines := categoryTipLines(l); len(lines) > 0 {
				// 「提示：」与它下面的内容同色（暗灰）：两行是一体的，标签单独上黄色反而
				// 把它从内容里摘出来（老版 2026-10-07 定）。
				fmt.Fprintf(&b, "  %s提示：%s\n", ansiDim, ansiReset)
				for _, line := range lines {
					fmt.Fprintf(&b, "    %s%s%s\n", ansiDim, line, ansiReset)
				}
			}
		} else if hints := exitCodeHintLine(l.FailCategories); hints != "" {
			// 关掉提示开关时保留旧行为：只给一行常见退出码含义。
			fmt.Fprintf(&b, "  %s%s%s\n", ansiYellow, hints, ansiReset)
		}
	}
	b.WriteString(bar + "\n")

	return Line{s: b.String()}
}

// ReportText 是 `report.txt` 的全文（不带色码：归档产物里不该有转义序列）。
//
// 输入全来自第三张纸与运行上下文那段密文：报告站是唯一解得开的地方，解出来的
// 命令行与清单路径**已经遮过秘密**，这里只管排版。
func ReportText(l contract.Ledger, body contract.RunMetaBody, maskedArgv []string, maskedManifest string) string {
	var b strings.Builder

	b.WriteString(reportHeadline + "\n")
	fmt.Fprintf(&b, "执行开始时间： %s\n", l.StartedAt.Format(reportTimeLayout))
	fmt.Fprintf(&b, "执行结束时间： %s\n", l.StoppedAt.Format(reportTimeLayout))
	fmt.Fprintf(&b, "\n【执行命令】 \n  %s\n", DisplayCommand(maskedArgv))

	b.WriteString("\n【执行参数】\n")
	fmt.Fprintf(&b, "  执行模式： %s\n", modeTitle(body.Mode))
	// 各模式自己的内容按模式分派——那是结构性差异，不是叫法（老版同一处也是分派）。
	switch body.Mode {
	case contract.ModeCommand:
		fmt.Fprintf(&b, "  执行命令： %s\n", body.Command)
	}
	fmt.Fprintf(&b, "  节点清单： %s\n", maskedManifest)
	fmt.Fprintf(&b, "  节点数量： %d\n", l.Total)
	fmt.Fprintf(&b, "  并发数： %d\n", body.Concurrency)
	if body.TimeoutConnect != 0 {
		fmt.Fprintf(&b, "  连接超时： %ds\n", body.TimeoutConnect)
	}
	if t := modeTimeout(body); t != 0 {
		// 措辞取自模式名字表（执行超时／传输超时／改密超时），与参数屏同一处来源。
		if label := timeoutLabel(body.Mode); label != "" {
			fmt.Fprintf(&b, "  %s： %ds\n", label, t)
		}
	}

	b.WriteString("\n【结果统计】\n")
	fmt.Fprintf(&b, "  总耗时：%.2f 秒\n", l.CostTime)
	if l.Interrupted {
		fmt.Fprintf(&b, "  节点总数：%d  完成总数：%d  未执行：%d（执行被中断）\n", l.Total, l.Done, l.NotRun)
	} else {
		fmt.Fprintf(&b, "  节点总数：%d  完成总数：%d  总数校验：%s\n", l.Total, l.Done, l.Verify)
	}
	fmt.Fprintf(&b, "  成功：%d    失败：%d\n", l.SuccessCounts, l.FailCounts)
	if len(l.FailCategories) > 0 {
		parts := make([]string, 0, len(l.FailCategories))
		for _, c := range l.FailCategories {
			parts = append(parts, fmt.Sprintf("%s ×%d", c.Category, c.Count))
		}
		fmt.Fprintf(&b, "  失败分类统计 -→  %s\n", strings.Join(parts, "  "))
		if body.ShowCategoryTips {
			if lines := categoryTipLines(l); len(lines) > 0 {
				b.WriteString("  提示：\n")
				for _, line := range lines {
					fmt.Fprintf(&b, "    %s\n", line)
				}
			}
		}
	}

	b.WriteString("\n【节点清单统计】\n")
	for _, group := range l.CategoryIPs {
		// 空的那一类不列：分类在、机器一个都没有时印一段空标题只会让人以为漏了名单。
		if len(group.IPs) == 0 {
			continue
		}
		fmt.Fprintf(&b, "\n%s（%d）：\n", group.Category, len(group.IPs))
		for _, ip := range group.IPs {
			fmt.Fprintf(&b, "%s\n", ip)
		}
	}
	return b.String()
}

// DisplayCommand 把 argv（含程序名）还原成一条**能照抄重跑**的命令行。
//
// 不能直接空格拼接：本机 shell 早把引号剥掉了，`-c 'who -b'` 到程序手里是
// `[-c "who -b"]`，平铺成 `-c who -b` 就是另一条命令。这里按「会不会被 shell 二次拆词」
// 逐段补回引号。
//
// 与执行站拼下发行那处**不是同一份**：那边一律整体单引号包起来、交给 `sh -c`；这一处
// 能省就省（逗号在内联清单里是分隔符，加了引号反而失真），为的是让人看着能照着敲。
func DisplayCommand(argv []string) string {
	parts := make([]string, 0, len(argv))
	for _, s := range argv {
		parts = append(parts, quoteForShell(s))
	}
	return strings.Join(parts, " ")
}

// shellSafeChars 不被 shell 二次解释的字符集：字母数字与 `_-./:=+,@%^`。
const shellSafeChars = "_-./:=+,@%^"

// quoteForShell 该加引号才加。
func quoteForShell(s string) string {
	if s == "" {
		return `''`
	}
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case strings.ContainsRune(shellSafeChars, r):
		default:
			return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
		}
	}
	return s
}

// categoryTipLines 汇总「提示：」块的内容：本次出现过的失败分类里写了解释的各出一行
// 「分类名：解释」，按传入次序（已按台数降序）。带退出码的那一类读到字段后换成固定说明，
// 不再解析分类名。
func categoryTipLines(l contract.Ledger) []string {
	out := make([]string, 0, len(l.FailCategories))
	for _, c := range l.FailCategories {
		tip := tipOf(l.Tables, c.Category)
		if c.ExitCode != nil {
			tip = exitCodeFailTip
			if meaning, ok := exitCodeHints[*c.ExitCode]; ok {
				tip += "。退出码 " + strconv.Itoa(*c.ExitCode) + " = " + meaning
			}
		}
		if tip == "" {
			continue
		}
		out = append(out, c.Category+"："+tip)
	}
	return out
}

// exitCodeHintLine 关掉提示开关时那一行：把带退出码的失败分类翻成常见含义，
// 按台数降序、同数按首现序（稳定）。
func exitCodeHintLine(categories []contract.CategoryCount) string {
	counts := map[int]int{}
	var order []int
	for _, c := range categories {
		if c.ExitCode == nil {
			continue
		}
		code := *c.ExitCode
		if _, ok := exitCodeHints[code]; !ok {
			continue
		}
		if _, seen := counts[code]; !seen {
			order = append(order, code)
		}
		counts[code] = c.Count
	}
	if len(order) == 0 {
		return ""
	}
	// 插入排序，稳定：同数保持首现序（老版就是这一支，不换成 sort.SliceStable 也一样，
	// 但换成包排序器就得多写一层比较函数的相等判定）。
	for i := 1; i < len(order); i++ {
		for j := i; j > 0 && counts[order[j-1]] < counts[order[j]]; j-- {
			order[j-1], order[j] = order[j], order[j-1]
		}
	}
	parts := make([]string, 0, len(order))
	for _, code := range order {
		parts = append(parts, fmt.Sprintf("%d >> %s", code, exitCodeHints[code]))
	}
	return "常见退出码：" + strings.Join(parts, "  ")
}

// tipOf 按分类名取判据文件里写的那句提示：两组都找，取第一条有内容的。
func tipOf(t contract.Tables, name string) string {
	for _, list := range [][]contract.Category{t.Categories, t.ExitCategories} {
		for _, c := range list {
			if c.Name == name && c.Tip != "" {
				return c.Tip
			}
		}
	}
	return ""
}

// isFallbackCategory 这个分类名是不是「判据表里压根没有」——那种显示的是报错原文本身，
// 归到「未归类」那一栏下面。
//
// 空名与「原因未知」不算：那是完全没信息，不是「有名字但没登记」。
func isFallbackCategory(t contract.Tables, name string) bool {
	if name == "" || name == CategoryUnknown {
		return false
	}
	for _, list := range [][]contract.Category{t.Categories, t.ExitCategories} {
		for _, c := range list {
			if c.Name == name {
				return false
			}
		}
	}
	return !IsExitCodeCategory(name)
}

// modeTimeout 这一模式那个超时值：命令与脚本是执行超时，上传下载是传输超时，改密也是
// 执行超时（老版同一个字段）。
func modeTimeout(body contract.RunMetaBody) int {
	switch body.Mode {
	case contract.ModeUpload, contract.ModeDownload:
		return body.TimeoutTransfer
	}
	return body.TimeoutExecute
}
