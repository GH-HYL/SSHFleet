package ssh

import "strings"

// 报文（message）：工具自己陈述事实的一行文本，形如
//
//	sshfleet.<环境>.<环节>: <英文描述>
//
// 这是**报文格式的唯一定义处**。判定层（internal/verdict）与各生成方都只经由本文件
// 产出报文，谁也不自己手拼前缀（ADR-0021「决定」第 2 条）。
//
// 为什么住在这里：报文要由执行侧（本包）与 batch 产出，而生成方都不许反过来依赖判定包；
// 本包又不依赖判定包。放在最底层、双方都已依赖的包，才不会有依赖环。
//
// # 格式
//
//   - `sshfleet` 是固定前缀：一眼看出这是工具报的，不是远端、不是系统；
//   - `<环境>` 是该报文所属的一摊，取值固定为下面 messageEnvs 那七种；
//   - `<环节>` 是该摊里的具体情形，小写，单词间用下划线；
//   - 冒号后接一句英文陈述。跟在 Go / 系统原始错误后面的，**原始错误原样保留、不吞不改**
//     （ADR-0021「决定」第 6 条：前缀是我们的，冒号后那段是别人的）。
//
// # 写法纪律
//
// 环节名只陈述**哪一步、什么现象**，不替使用者下结论：用 `finished`（传完了）、
// `canceled`（被取消）这类陈述现象的写法，**不用 `partial` / `failed`**。叫不叫「部分成功」
// 是配置里那条分类的 `name` 说了算（ADR-0021「决定」第 8 条第 2 款）。
//
// # 不做译文层
//
// 报文原文就是使用者看到的文本：终端明细、output.txt、report.txt、results.xlsx 与执行
// 日志一律原样呈现，前缀不摘、术语不翻。给人看的中文只有配置里的 `name` 与 `tip`。
// 别把这里当成漏翻的缺陷补一层翻译回去——多一层译文等于给同一句话养两份说法
// （spec《Further Notes》最后一条理由）。
const messagePrefix = "sshfleet."

// messageEnvs 环境取值（七种，ADR-0021「决定」第 1 条）。写错的环境名不产出合法报文，
// 好让它在产出的那一刻就露出坏形状，而不是静默混进匹配文本。
var messageEnvs = map[string]bool{
	"connect":  true, // 建连与认证
	"session":  true, // 会话与终端
	"run":      true, // 命令执行
	"transfer": true, // 传输
	"passwd":   true, // 批量改密
	"answer":   true, // 代填
	"abort":    true, // 中断与取消
}

// Message 拼一行报文：`sshfleet.<env>.<stage>: <detail>`。
//
// detail 是英文陈述；跟在原始错误后面时就把原文原样传进来。env 不在七种取值里时**照样
// 拼出来**（前缀仍在、形状不合法），由用例与人的眼睛发现——不 panic、不静默改写，
// 因为产出报文这一刻多半已经在处理一个失败了，不能再让工具自己崩掉。
func Message(env, stage, detail string) string {
	if messageEnvs[env] {
		return messagePrefix + env + "." + stage + ": " + detail
	}
	// 不接受的环境：前缀照留，环境段带上一个非法字符，形状校验一眼能看出来。
	// 不 panic——产出报文这一刻多半已经在处理一个失败，工具不能因此再崩一次。
	return messagePrefix + env + "!" + "." + stage + ": " + detail
}

// IsMessage 这段文本里出现报文没有（以 `sshfleet.` 开头的一行）。
//
// 用途有二：成功规则里「报错原文出现报文即不算成功」（一级定性删掉之后，这是拦住
// 「代填没送完但命令退出 0」被判成成功的唯一办法）；以及排查时按前缀认出工具报文。
func IsMessage(text string) bool {
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(line, messagePrefix) {
			return true
		}
	}
	return false
}
