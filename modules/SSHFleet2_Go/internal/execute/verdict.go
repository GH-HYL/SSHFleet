package execute

// 定分类：这一台跑完之后归到哪一类。
//
// 判据表随第一张纸来（ADR-0035 决定五），执行站不读文件；两组的分工只看**目的一条命令
// 有没有给出退出码**——给了走第二组（会话被拒、命令自身失败），没给走第一组（连不上、
// 超时、认证失败）。顺序即优先级，命中即停。
//
// 匹配口径照老版写死（`verdict/keywords.go:177-223`），**不读 `[interactive]` 那两格**：
// 那两格服务的是代填与改密，与分类无关；执行站也不回头读配置（跨站回取）。
//
// 「成功」也算一类：那一类的名字来自模式名字表（`message.SuccessCategory`）。它不是结论格
// ——这一格里装的都是分类名，算不算成功由结算站拿本模式的成功分类去对（ADR-0035 决定四）。

import (
	"strings"

	"sshfleet2/internal/contract"
	"sshfleet2/internal/message"
)

// fallbackMaxLen 兜底分类名的最大长度：判据表一条都没命中时，分类名就用报错原文本身，
// 超过这个长度截断——整段长报文会把统计块与报告撑爆。按 rune 数，不按字节：按字节切会把
// 多字节字符切成无效 UTF-8（老版 2026-09-14 修过这一处）。
const fallbackMaxLen = 200

// categoryOf 这一台的分类名。入参是收齐了事实的那一条结果。
//
// 三步，顺序照老版：
//  1. 按本模式的成功规则干净走完 → 成功那一类；
//  2. 有退出码 → 先查第二组；没认出、且退出码非 0 → 按退出码给「执行失败(退出码N)」；
//  3. 其余 → 查第一组；没认出就把原文当分类名（保留具体失败内容），两段都空才兜底「原因未知」。
func categoryOf(mode contract.Mode, t contract.Tables, r *contract.NodeResult) string {
	if ranClean(mode, r) {
		return message.SuccessCategory(mode)
	}

	// 这一格装着工具自己的报文、远端原话、服务端提示三样（见 contract.NodeResult.ErrorText）。
	src := r.ErrorText

	if r.ExitCode != nil {
		if hit := firstCategory(t.ExitCategories, joinText(src, r.Output)); hit != "" {
			return hit
		}
		if *r.ExitCode != 0 {
			return message.CategoryOfExitCode(*r.ExitCode)
		}
		// 退出码 0 却不算成功（代填没送完这一类）：失败原因不在退出码里，按第一组的规矩走——
		// 原文在前、查表、认不出就显示原文。此时给「执行失败(退出码0)」既自相矛盾，
		// 又会把真正的原文盖掉。
	}

	if src != "" {
		if hit := firstCategory(failCategories(mode, t), src); hit != "" {
			return hit
		}
		if trimmed := strings.TrimSpace(src); trimmed != "" {
			return truncateCategory(trimmed)
		}
	}
	// 改密不拿输出原文匹配：「密码过期」那句提示本身就是它的前提，把提示当失败原因
	// 会把干净成功也归到失败类里（老版同口径：改密且已见过入场信号时只拿报错原文匹配）。
	if mode != contract.ModePasswd && r.Output != "" {
		if hit := firstCategory(t.Categories, r.Output); hit != "" {
			return hit
		}
		if trimmed := strings.TrimSpace(r.Output); trimmed != "" {
			return truncateCategory(trimmed)
		}
	}
	return message.CategoryUnknown
}

// ranClean 本模式的成功规则。命令与脚本共用一条（老版 `verdict.succeeded`）：
// 退出码为 0、没超时没取消，**且报错原文里没有工具自己的报文**。
//
// 最后那一条不是多余的：报文只在非成功形态下写，而「代填没送完但命令退出 0」这类情形
// 退出码是 0——没有它就会被判成成功。
//
// 上传与下载共用一条：连上了、失败文件数为 0、成功文件数 > 0、没超时没取消。
//
// 改密单列一条：干净收场（没超时没取消、报错原文为空、下发的 `:` 不是非 0）**且**整场
// 出现过入场信号——没见过信号的那些台，收尾时已经写了一句「密码未过期」，报错原文不为空，
// 于是自然落不进成功那一类。
func ranClean(mode contract.Mode, r *contract.NodeResult) bool {
	switch mode {
	case contract.ModeCommand, contract.ModeScript:
		return r.ExitCode != nil && *r.ExitCode == 0 && !r.TimedOut && !r.Cancelled &&
			!isMessage(r.ErrorText)
	case contract.ModeUpload, contract.ModeDownload:
		return r.Connected && r.Transfer.FailFiles == 0 && r.Transfer.SuccessFiles > 0 &&
			!r.TimedOut && !r.Cancelled
	case contract.ModePasswd:
		if !r.Connected || r.TimedOut || r.Cancelled || r.ErrorText != "" {
			return false
		}
		for _, s := range r.Steps {
			if s.Name == contract.StepPasswd && s.ExitCode != nil && *s.ExitCode != 0 {
				return false
			}
		}
		return true
	}
	return false
}

// isMessage 这段原文里有没有工具自己写的报文（`sshfleet.<环境>.<环节>: …`）。
//
// 按行判前缀，不按子串：报文的定义就是「这一行的开头是 `sshfleet.`」；整段里搜子串会把
// 远端命令输出里恰好打印了这几个字的那种误认成报文。
func isMessage(text string) bool {
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(line, messagePrefix) {
			return true
		}
	}
	return false
}

// failCategories 这一模式按哪几张表认失败分类：顺序即优先级，命中即停。
//
// 改密的专用表排在最前。它说的是「改密对话跑了、没改成」的话（新密码不合规、写不进
// shadow、早收场），本来就不该跟「连不上」「会话被拒」抢；排前面，早收场时工具自己那
// 句报文才会先被「改密未完成」认领，而不是落到通用的「密码过期」上。
//
// 其余模式只用通用那一组：专用表里的措辞跟命令、传输无关，拿它来认只会认错。
//
// 两张表是两份文件、两个字段（`keywords_error.conf` 与 `keywords_passwd.conf`），
// 在这里接成一条只是为了「谁先谁后」这一件事，不改变它们各自的归属。
func failCategories(mode contract.Mode, t contract.Tables) []contract.Category {
	if mode != contract.ModePasswd || len(t.PasswdCategories) == 0 {
		return t.Categories
	}
	merged := make([]contract.Category, 0, len(t.PasswdCategories)+len(t.Categories))
	merged = append(merged, t.PasswdCategories...)
	return append(merged, t.Categories...)
}

// firstCategory 在一组分类里找第一条命中的，返回它的名字；一条都没命中返回空串。
//
// 块内自上而下遍历分类、分类内按关键词次序试——**顺序就是优先级**，先命中的胜出，
// 所以判据表里越具体的原因越要写在前面。
func firstCategory(items []contract.Category, text string) string {
	if len(items) == 0 || text == "" {
		return ""
	}
	lower := strings.ToLower(text) // 大小写不敏感：两边一起转小写再比
	for _, c := range items {
		for _, kw := range c.Keywords {
			if keywordHit(strings.ToLower(kw), lower) {
				return c.Name
			}
		}
	}
	return ""
}

// keywordHit 单条关键词命中判定：带 `*` 的按 `*` 切片段、顺序查找，其余就是普通子串包含。
//
// 星号是判据表里唯一的通配符，含义只有一句「这里可以是任意长度的内容」；切出来的空片段
// （连续两个星号、或以星号开头结尾）不参与查找，否则一个 `*` 只能匹到空串。
func keywordHit(keyword, text string) bool {
	if !strings.Contains(keyword, "*") {
		return strings.Contains(text, keyword)
	}
	pos := 0
	for _, part := range strings.Split(keyword, "*") {
		if part == "" {
			continue
		}
		idx := strings.Index(text[pos:], part)
		if idx < 0 {
			return false
		}
		pos += idx + len(part)
	}
	return true
}

// joinText 把两段原文合成一段待匹配文本，中间用换行隔开。
//
// 判据都是固定文案，合成一段只影响「能不能找到」，不影响找到的是哪一类——分类的先后由
// 判据表决定。用换行而不是空格：报文的环节名与远端原话各自成行，粘成一个词反而更容易误伤。
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

// truncateCategory 把兜底用的原文截到 fallbackMaxLen 个字符（按 rune 数），末尾加一个省略号。
func truncateCategory(text string) string {
	r := []rune(text)
	if len(r) <= fallbackMaxLen {
		return text
	}
	return string(r[:fallbackMaxLen]) + "…"
}
