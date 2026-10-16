// Package verdict 承载结果判定：执行侧只记录事实，成败与分类只在这里产生一次。
//
// 术语见 GLOSSARY.md「结果判定（verdict）」；设计依据与逐条决策见
// docs/issues/result-verdict/spec.md（两级判定 D6、选组判据 4.3、成功规则 4.5）。
//
// 依赖边界：本包只允许引用 ssh（结果结构体）与 cli（模式枚举）——
// batch / output 反向引用本包，判定绝不回头依赖它们。
package verdict

import (
	"strconv"
	"strings"

	"sshfleet/internal/cli"
	"sshfleet/internal/ssh"
)

// 成败二态（D1）：成功 / 其他（其他即失败）。
// 「部分成功」只是 Other 里的一个分类名，不是第三态。
//
// 结果结构体住 ssh，而 ssh 不能反向引用本包（本包引它），所以 Verdict 字段是裸 string、
// 这里只给常量不作类型——判定与消费方一律用这两个常量比较，不写裸字面量。
const (
	// Success 执行 / 传输按该模式的成功规则走完了。
	Success = "success"
	// Other 成败二态的另一侧：一切没走成功的都是它。
	Other = "other"
)

// 一级定性给出的分类名：结构事实直接成立，不经判据表。
// 这些名字与 config/keywords_error.conf、keywords_passwd.conf 里的同名分类对应——
// 改名会让终端统计把它们当兜底原文单排一行。
const (
	// CategoryPasswordExpired 中止词命中的分类名（GLOSSARY「中止词」：归类「密码过期」是约定）。
	// 导出：它同时是对外契约——改密的入场信号与守卫都按这个分类名取关键词
	// （见 Keywords.PasswordExpired），名字只活在这一处。
	CategoryPasswordExpired = "密码过期"
	catTriggerMiss          = "触发词未命中"  // 代填还有没送出去的
	catExhaustedTimeout     = "代填用尽后超时" // 代填全部送出后仍等到超时
	catPartialSuccess       = "部分成功"    // 传输有成功有失败
	catPasswdNotExpired     = "密码未过期"   // 改密整场没出现过期信号
	// 改密卡住的四种形态（2026-10-14 由结构事实定性，此前是执行侧编的四句
	//「改密没有完成：…」文案 + 判据表关键词认回来）。两个布尔量就够了：
	// PasswdFedAny＝工具有没有认出远端在问什么并答过一句；TimedOut＝是不是等到点。
	catPasswdNoPrompt = "提示词没认出" // 一句都没喂出去、也没等到点：远端问的话提示词表里没有
	catPasswdNoSignal = "没等到提示"  // 一句都没喂出去、等到点了：远端没出声，或问的话没认出来
	catPasswdNoReply  = "远端无响应"  // 喂进去了、等到点也没下文（多半卡在读写密码文件）
	catPasswdGiveUp   = "改密未完成"  // 喂进去了、远端还在重问而已给的答案用完；也是这一族的兜底
	// 外部中断（Ctrl+C / 上级取消）按"中断在哪一步"分两档：连接都没建成的机器
	// 压根没被执行到，跟"跑到一半被中断"混为一谈会让人以为它跑过。
	catCanceled        = "任务已取消" // 已连上，命令 / 传输 / 改密跑到中途被中断
	catConnectCanceled = "建连被取消" // 还在建立连接就被中断（连接慢的机器最容易撞上）
)

// 判据表里那条按退出码自动给的兜底分类的前缀（配置文件写不了带数字的名字）。
const exitCodeFailPrefix = "执行失败(退出码"

// Judge 单条结果的判定入口（单入口，内部先一级后二级，D6）：
// 按模式选成功规则（4.5），定出成败、失败分类与定论退出码，写进结论区三格。
//
// 纯函数（D8）：只看入参（结果 + 模式 + 判据表），不读全局状态。
// 结论区只由这里写（只此一次）；事实区一个字都不碰。
func Judge(res *ssh.Result, mode cli.Mode, kw *Keywords) {
	res.Verdict = Other
	res.Category = ""
	res.ExitCode = nil

	// —— 一级：结构事实直接定性（不查表，4.1）——
	switch mode {
	case cli.ModeCommand, cli.ModeScript:
		// 会话里执行命令的三类（命令 / 脚本 / 代填）共用同一条成功规则（4.5）：
		// 没配代填时 AnswersMissed / AbortLine 天然为空，条件自然退化。
		if res.AbortLine != "" {
			res.Category = CategoryPasswordExpired
			res.ExitCode = res.CommandExitCode // 中止即收场，通常是 nil
			return
		}
		if res.SessionBegun && !res.Canceled && len(res.AnswersMissed) > 0 {
			res.Category = catTriggerMiss
			res.ExitCode = res.CommandExitCode // 脚本自己跑完了：定论退出码照实给（0 也是它）
			return
		}
		if res.SessionBegun && res.AnswersExhausted && res.TimedOut {
			res.Category = catExhaustedTimeout
			return
		}
		if res.SessionBegun && res.CommandExitCode != nil && *res.CommandExitCode == 0 &&
			!res.TimedOut && !res.Canceled {
			res.Verdict = Success
			res.ExitCode = res.CommandExitCode
			return
		}

	case cli.ModeUpload, cli.ModeDownload:
		// 有成功有失败是结构性状态，先于失败原因展示（与既有顺序一致）
		if res.FailedFiles > 0 && res.SuccessFiles > 0 {
			res.Category = catPartialSuccess
			return
		}
		if res.ConnectSuccess && res.FailedFiles == 0 && res.SuccessFiles > 0 &&
			!res.TimedOut && !res.Canceled {
			res.Verdict = Success
			return
		}

	case cli.ModePasswd:
		// 「干净收场」＝会话正常结束、没超时没中断没早收场、没有报错，
		// **且 passwd 自己也不是以非 0 收的场**——它非 0 就是"没改成"，退码归在 Steps 里
		//（StepPasswd）。此前这一条藏在 Error 的兜底文案里（执行侧会为它编一句
		//「改密没有完成」），2026-10-14 改成读结构事实，不再依赖那句话。
		cleanStop := !res.PasswdEarlyClose && !res.TimedOut && !res.Canceled &&
			derefString(res.Error) == "" && !passwdExitedNonzero(res)
		if cleanStop {
			if res.EnterSignalSeen {
				res.Verdict = Success
				return
			}
			// 整场没出现过期信号 = 这台机器不需要改密（原执行侧收尾判定搬到这里）
			res.Category = catPasswdNotExpired
			return
		}
		// 卡住了：按结构事实定性，不看 Error 里的文案（D3）。
		//
		// 两类分开：
		//   · 超时 / 早收场——**无条件覆盖**文案。这两种形态下 Error 里那点内容多半是
		//     「命令执行超时(5s)」这类通用描述，照它分类只会给出「执行超时」，盖掉
		//     "改密卡在哪一步"这个更有用的信息（原执行侧也是无条件覆盖的）。
		//   · 其余（passwd 自己非 0 收场）——远端说了原因就让位给判据表，按它的措辞分；
		//     没说话才用兜底的名。
		if !res.Canceled {
			switch {
			case res.PasswdEarlyClose:
				res.Category = catPasswdGiveUp
				return
			case res.TimedOut:
				if res.PasswdFedAny {
					res.Category = catPasswdNoReply
				} else {
					res.Category = catPasswdNoSignal
				}
				return
			case derefString(res.Error) == "" && !res.PasswdFedAny:
				// passwd 非 0 收场又一言不发，且工具没喂出去过：提示词表没认出远端在问什么
				res.Category = catPasswdNoPrompt
				return
			case derefString(res.Error) == "" && passwdExitedNonzero(res):
				// 同上但喂过了：远端收了密码却没下文
				res.Category = catPasswdGiveUp
				return
			}
		}
	}

	// —— 二级之前：外部中断（Ctrl+C / 上级取消）——
	//
	// 这是**结构事实**，不看报错文案（与 TimedOut 同口径，见 ssh.endFacts 的说明）。
	// 排在模式规则之后：中止词（密码过期）那类结论比"被中断"更具体，不该被盖掉；
	// 排在各模式的成功规则之后：取消本来也走不到成功。传输的「部分成功」同理更具体。
	if res.Canceled {
		if res.ConnectSuccess {
			res.Category = catCanceled
		} else {
			res.Category = catConnectCanceled
		}
		return
	}

	// —— 二级：文案查判据表（4.3 选组）——
	// 选组是纯字段判定：目的那条命令真的给了退出码 → 第二组；否则第一组。
	// 改密已见过期信号的会话不带服务端提示参与匹配——那句提示是改密的前提，不是失败原因
	//（原执行侧"改密对话走起来之后不并 banner"的口径，随字段独立移到消费侧）。
	src := joinText(res.ServerBanner, derefString(res.Error))
	if mode == cli.ModePasswd && res.EnterSignalSeen {
		src = derefString(res.Error)
	}
	if res.CommandExitCode != nil {
		if hit := kw.matchText(true, joinText(src, res.Output)); hit != "" {
			res.Category = hit
		} else {
			res.Category = exitCodeFailPrefix + strconv.Itoa(*res.CommandExitCode) + ")"
		}
		res.ExitCode = res.CommandExitCode
		return
	}
	res.Category = classifyByKeywords(kw, src, res.Output)
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
// 这个分类名由 Judge 产出（见上面那个分支），产出时退出码同时也写进了 Result.ExitCode——
// 所以消费方要那个数字，读字段即可，不必再拿正则从名字里解回来（2026-10-09）。
func IsExitCodeCategory(category string) bool {
	return strings.HasPrefix(category, exitCodeFailPrefix)
}

// passwdExitedNonzero 改密下发的 `:` 是不是以非 0 收的场（那个码记在 Steps 的 StepPasswd 上）。
//
// 它是「这次改密到底成没成」的关键事实之一：passwd 非 0 就是没改成，哪怕它一个字都没说。
// 执行侧不再为这种情况编一句「改密没有完成」（D3），改由这里读结构字段判断。
func passwdExitedNonzero(res *ssh.Result) bool {
	for _, s := range res.Steps {
		if s.Name == ssh.StepPasswd && s.ExitCode != nil && *s.ExitCode != 0 {
			return true
		}
	}
	return false
}

// IsFallbackCategory 判断分类名是否为「关键词未命中时的兜底原文」。
// 必须覆盖判据表两组的名字（Has 取并集），漏掉一组会把覆盖出来的分类误判成兜底原文。
//
// 另外要放过**由代码按结构事实定性、配置表里没有登记**的那几个名字——判据表是
// 「文案 → 分类」的表，而这些分类压根不是查表得来的，不放过就会被上屏成「未归类」。
func IsFallbackCategory(category string, kw *Keywords) bool {
	if category == "" || category == Unclassified {
		return false
	}
	if kw.Has(category) {
		return false
	}
	switch category {
	case catPartialSuccess,
		catPasswdNoPrompt, catPasswdNoSignal, catPasswdNoReply, catPasswdGiveUp:
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
