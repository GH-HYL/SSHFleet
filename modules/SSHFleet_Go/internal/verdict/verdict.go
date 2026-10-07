// Package verdict 承载结果判定：执行侧只记录事实，成败与分类只在这里产生一次。
//
// 术语见 CONTEXT.md「结果判定（verdict）」；设计依据与逐条决策见
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
	catPasswordExpired  = "密码过期"    // 中止词命中（CONTEXT「中止词」：归类「密码过期」是约定）
	catTriggerMiss      = "触发词未命中"  // 代填还有没送出去的
	catExhaustedTimeout = "代填用尽后超时" // 代填全部送出后仍等到超时
	catPartialSuccess   = "部分成功"    // 传输有成功有失败
	catPasswdNotExpired = "密码未过期"   // 改密整场没出现过期信号
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
			res.Category = catPasswordExpired
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
		cleanStop := !res.PasswdEarlyClose && !res.TimedOut && !res.Canceled && derefString(res.Error) == ""
		if cleanStop {
			if res.EnterSignalSeen {
				res.Verdict = Success
				return
			}
			// 整场没出现过期信号 = 这台机器不需要改密（原执行侧收尾判定搬到这里）
			res.Category = catPasswdNotExpired
			return
		}
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

// IsFallbackCategory 判断分类名是否为「关键词未命中时的兜底原文」。
// 必须覆盖判据表两组的名字（Has 取并集），漏掉一组会把覆盖出来的分类误判成兜底原文。
func IsFallbackCategory(category string, kw *Keywords) bool {
	if category == "" || category == Unclassified {
		return false
	}
	if kw.Has(category) {
		return false
	}
	if category == catPartialSuccess {
		return false
	}
	return !strings.HasPrefix(category, exitCodeFailPrefix)
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
