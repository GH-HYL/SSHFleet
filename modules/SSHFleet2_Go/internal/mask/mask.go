// Package mask 是横切叶子包：认出秘密、把它遮住；清单的脱敏副本也由它写。
//
// **遮只有一种形态**：前 2 + `****` + 后 2，四个字符以内整段抹（`Secret`）。遮一个值、
// 遮一条内联记录、把明文从一段采集文本里擦掉、写清单脱敏副本，四件事都从这一条出来；
// 别处不许自己拼星号——结构检查类七盯着（建站 07 规格第六节）。
//
// 谁要谁现算，不产生字段、不往下传——把「已经遮好的值」装进纸里等于凭空多一格，
// 还让下游以为那就是原值（ADR-0032 第四条）。
package mask

import "strings"

// maskToken 是遮掉之后顶替那几个字符的东西。
//
// 屏幕、内联清单、`report.txt` 三处遮出来的样子必须是同一串，所以只许这一份：三处各写
// 一次的话，改一处漏两处，同一个密码在屏上是一个样、在报告里是另一个样。
const maskToken = "****"

// Secret 把一个秘密遮成认得出形状、看不出内容的样子。
//
// 四个字符以内整个遮掉；再长的留前两个、后两个——既让人能对上「是哪一条」，
// 又不足以还原。与老版同一形态（老版 common.MaskSecret），一个字不改。
func Secret(s string) string {
	r := []rune(s)
	if len(r) <= 4 {
		return maskToken
	}
	return string(r[:2]) + maskToken + string(r[len(r)-2:])
}

// maskCredentialFields 把一条节点记录里的两个凭据段（第 4 段密码、第 6 段口令）遮掉。
//
// 内联清单与清单副本共用这一处：段号只在这里写一次，两处各判各的空（空段不动），
// 遮法仍是 `Secret`。
func maskCredentialFields(fields []string) {
	for _, idx := range []int{3, 5} {
		if idx < len(fields) && strings.TrimSpace(fields[idx]) != "" {
			fields[idx] = Secret(fields[idx])
		}
	}
}

// Scrub 把本次喂进去的凭据从一段采集文本里擦掉。
//
// 改密时远端 PTY 什么时候关回显由 passwd 说了算：我们写进去的字节可能正好落在它关回显
// 之前那一瞬，被终端原样回显出来。这份文本会跟着结果进归档、报告与明细表，所以在采到之后
// 先擦一遍，不指望远端。
//
// 擦成什么样子仍归本包一处：与 `Secret` 同一个形态。短密码可能与输出里的无关文字重合——
// 两害相权，宁可把它也遮掉。
func Scrub(text string, secrets ...string) string {
	for _, s := range secrets {
		if s == "" {
			continue
		}
		text = strings.ReplaceAll(text, s, Secret(s))
	}
	return text
}

// CommandLine 把整条命令行里带凭据的那几段遮掉，其余原样留着。
//
// 三个位置会带秘密：`--change-password` 的取值（改密模式的新密码，加密时是密文文件路径，
// 两种都得遮）、`-f` 的内联清单（`-f 1.2.3.4,22,root,pw` 把密码写在命令行里）、以及
// `-f=` 这种等号写法。逐段判，不按位置猜——`-c 'echo -f 1.2.3.4,...'` 里的那段是正文，
// 不是清单。
//
// 报告与日志里贴命令行只能贴这一版（老版 common.MaskCommandLine 同口径）。
func CommandLine(argv []string) []string {
	out := make([]string, len(argv))
	maskNext := false
	for i, a := range argv {
		switch {
		case maskNext:
			out[i] = Secret(a)
			maskNext = false
		case a == "--change-password":
			out[i] = a
			maskNext = true
		case strings.HasPrefix(a, "--change-password="):
			out[i] = "--change-password=" + Secret(strings.TrimPrefix(a, "--change-password="))
		case looksLikeInlineList(a):
			out[i] = InlineRecord(a)
		case strings.HasPrefix(a, "-f=") && looksLikeInlineList(strings.TrimPrefix(a, "-f=")):
			out[i] = "-f=" + InlineRecord(strings.TrimPrefix(a, "-f="))
		default:
			out[i] = a
		}
	}
	return out
}
