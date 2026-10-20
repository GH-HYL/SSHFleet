// Package verdict 承载结果判定：执行侧只记录事实，成败与分类只在这里产生一次。
//
// 术语见 GLOSSARY.md「结果判定（verdict）」；报文时代的判定口径见 ADR-0021
// 「决定」第 3 / 8 条与 docs/issues/message-based-classification/spec.md。
//
// 依赖边界：本包只允许引用 ssh（结果结构体、报文）与 cli（模式枚举）——
// batch / output 反向引用本包，判定绝不回头依赖它们。
//
// # 判定层不做的事
//
// 它**不定义分类名**：一个失败归到哪一类，只由判据表说了算。结构事实（成了几个、
// 有没有超时、代填喂进去过没有）由执行侧在收尾时说成一句报文，与远端原话走同一套查表。
// 读结构字段只有一个用途——按各模式的成功规则判成败。
//
// 唯一一处例外是 `执行失败(退出码N)`：那个名字里带数字，配置的 `name` 写不了变量，
// 做不成可匹配的关键词，所以由这里按退出码拼出。别的地方一个都不许有。
package verdict

import (
	"strconv"
	"strings"

	"sshfleet/internal/cli"
	"sshfleet/internal/ssh"
)

// 成败二态（D1）：成功 / 其他（其他即失败）。
//
// 结果结构体住 ssh，而 ssh 不能反向引用本包（本包引它），所以 Verdict 字段是裸 string、
// 这里只给常量不作类型——判定与消费方一律用这两个常量比较，不写裸字面量。
const (
	// Success 执行 / 传输按该模式的成功规则走完了。
	Success = "success"
	// Other 成败二态的另一侧：一切没走成功的都是它。
	Other = "other"
)

// 判据表里那条按退出码自动给的兜底分类的前缀（配置文件写不了带数字的名字）。
//
// 这是本包唯一一处「代码给分类名」：名字里带一个数字，做不成报文可匹配的关键词。
const exitCodeFailPrefix = "执行失败(退出码"

// Judge 单条结果的判定入口：按模式选成功规则（定成败），失败分类一律交判据表。
//
// 纯函数（D8）：只看入参（结果 + 模式 + 判据表），不读全局状态。
// 结论区只由这里写（只此一次）；事实区一个字都不碰。
//
// 结构字段只用来判成败，不再用来起名——那批「按结构事实直接定性的分类」随报文体系退役
// （ADR-0021 决定第 3 条）。
func Judge(res *ssh.Result, mode cli.Mode, kw *Keywords) {
	res.Verdict = Other
	res.Category = ""
	res.ExitCode = nil

	// —— 成功规则（读结构字段只为这一件事）——
	if succeeded(res, mode) {
		res.Verdict = Success
		res.ExitCode = res.CommandExitCode
		return
	}

	// —— 失败分类：一律查表（选组规则不变，4.3）——
	res.Category = classify(res, mode, kw)
}

// succeeded 各模式的成功规则。
//
// 三条口径：
//   - 会话里跑命令（命令 / 脚本 / 代填共用一条）：退出码为 0、没超时没中断，
//     **且报错原文里没有报文**。最后这一条是报文体系新增的：一级定性删掉之后，
//     「代填没送完但命令退出 0」不再有分支抢在成功规则前面，没有它就判成成功了。
//   - 上传 / 下载：连接成功、一个文件都没失败、至少成一个、没超时没中断。
//     （部分成功时 FailedFiles > 0，自然不成立，不必另判。）
//   - 改密：干净收场且整场出现过入场信号。干净收场里那句「报错原文为空」保留——
//     报文只在非成功形态下写，它读作「没有事实要陈述」，不是第二条给分类名的路径。
func succeeded(res *ssh.Result, mode cli.Mode) bool {
	switch mode {
	case cli.ModeCommand, cli.ModeScript:
		return res.SessionBegun && res.CommandExitCode != nil && *res.CommandExitCode == 0 &&
			!res.TimedOut && !res.Canceled && !ssh.IsMessage(derefString(res.Error))

	case cli.ModeUpload, cli.ModeDownload:
		return res.ConnectSuccess && res.FailedFiles == 0 && res.SuccessFiles > 0 &&
			!res.TimedOut && !res.Canceled

	case cli.ModePasswd:
		// 干净收场：会话正常结束、没超时没中断没早收场、没有报错，
		// 且 passwd 自己也不是以非 0 收的场——它非 0 就是"没改成"，退码归在 Steps 里。
		if res.PasswdEarlyClose || res.TimedOut || res.Canceled {
			return false
		}
		if derefString(res.Error) != "" || passwdExitedNonzero(res) {
			return false
		}
		return res.EnterSignalSeen
	}
	return false
}

// classify 选组 + 查表。失败时一定给出一个分类名：命中即分类名，没命中即原文本身
// （与今天「未归类」的口径一致），两段都空才兜底「原因未知」。
//
// 选组是纯字段判定（4.3）：目的那条命令真的给了退出码 → 第二组；否则第一组。
// 改密已见过期信号的会话不带服务端提示参与匹配——那句提示是改密的前提，不是失败原因。
func classify(res *ssh.Result, mode cli.Mode, kw *Keywords) string {
	src := joinText(res.ServerBanner, derefString(res.Error))
	// 代填的中止词命中原文并入待匹配文本：中止确实意味着密码过期，但分类名不该由代码给，
	// 让它跟其他原文一样去查表（ADR-0021 决定第 9 条）。
	src = joinText(src, res.AbortLine)
	if mode == cli.ModePasswd && res.EnterSignalSeen {
		src = derefString(res.Error)
	}
	if res.CommandExitCode != nil {
		// 定论退出码照实给：命令真跑过，它自己的退出码就是结论的一部分（0 也是它）。
		// 这一步原先散在「代填未命中」那条一级分支里，一级分支删掉后收拢到这里。
		res.ExitCode = res.CommandExitCode
		if hit := kw.matchText(true, joinText(src, res.Output)); hit != "" {
			return hit
		}
		// 退出码 0 却判失败（例如代填没送完）：失败原因不在退出码里，
		// 拼一个「执行失败(退出码0)」既自相矛盾、也会把真正的报文原文盖掉。
		// 让它照第一组的规矩走——原文在前、再查表、认不出就显示原文。
		if *res.CommandExitCode != 0 {
			return exitCodeFailPrefix + strconv.Itoa(*res.CommandExitCode) + ")"
		}
	}
	return classifyByKeywords(kw, src, res.Output)
}

// classifyByKeywords 第一组匹配：先报错原文（含服务端提示）、后输出，都未命中时
// 把原文本身作为分类（保留具体失败内容），两段均空才兜底「原因未知」。
func classifyByKeywords(kw *Keywords, src, output string) string {
	if src != "" {
		if hit := kw.matchText(false, src); hit != "" {
			return hit
		}
		if trimmed := strings.TrimSpace(src); trimmed != "" {
			return truncate(trimmed)
		}
	}
	if output != "" {
		if hit := kw.matchText(false, output); hit != "" {
			return hit
		}
		if trimmed := strings.TrimSpace(output); trimmed != "" {
			return truncate(trimmed)
		}
	}
	return Unclassified
}

// IsExitCodeCategory 分类名是不是「按退出码自动给的兜底分类」（执行失败(退出码N)）。
//
// 这个分类名由 Judge 产出，产出时退出码同时也写进了 Result.ExitCode——
// 所以消费方要那个数字，读字段即可，不必再拿正则从名字里解回来（2026-10-09）。
func IsExitCodeCategory(category string) bool {
	return strings.HasPrefix(category, exitCodeFailPrefix)
}

// passwdExitedNonzero 改密下发的 `:` 是不是以非 0 收的场（那个码记在 Steps 的 StepPasswd 上）。
//
// 它是「这次改密到底成没成」的关键事实之一：passwd 非 0 就是没改成，哪怕它一个字都没说。
func passwdExitedNonzero(res *ssh.Result) bool {
	for _, s := range res.Steps {
		if s.Name == ssh.StepPasswd && s.ExitCode != nil && *s.ExitCode != 0 {
			return true
		}
	}
	return false
}

// IsFallbackCategory 判断分类名是否为「关键词未命中时的兜底原文」。
//
// 只有一条规则：判据表里登记过这个名字，就是已知分类。判定包不另养名单——
// 名单与配置是同一个事实的两份副本，而配置侧还维护着 tip、改得更勤；副本一脱节，
// 使用者照着配置改分类就"改了不生效"（2026-10-18 就这么漏过一次）。
func IsFallbackCategory(category string, kw *Keywords) bool {
	if category == "" || category == Unclassified {
		return false
	}
	if kw.Has(category) {
		return false
	}
	return !IsExitCodeCategory(category)
}

// Unclassified 完全无信息时的兜底分类。
const Unclassified = "原因未知"

// fallbackMaxLen 兜底原文的最大长度（超出截断，避免长报文撑爆统计/报表）。
const fallbackMaxLen = 200

func derefString(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// joinText 合成判据匹配文本：判据都是服务端固定文案，合并两段只影响"能不能找到"，
// 不影响找到的是哪一类（分类顺序由判据文件决定）。
func joinText(a, b string) string {
	switch {
	case a == "":
		return b
	case b == "":
		return a
	default:
		return a + "\n" + b
	}
}

func truncate(text string) string {
	if len(text) <= fallbackMaxLen {
		return text
	}
	// 按 rune 截断：按字节切会把多字节字符切成无效 UTF-8（2026-09-14 审计修复）
	r := []rune(text)
	out := make([]rune, 0, fallbackMaxLen)
	count := 0
	for _, c := range r {
		if count >= fallbackMaxLen {
			break
		}
		out = append(out, c)
		count++
	}
	return string(out) + "…"
}
