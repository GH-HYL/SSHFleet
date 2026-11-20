package message

import "fmt"

// 两个取值标签：「当前值：」与「取值非法：」。
//
// 它们原先散在十来处 `fmt.Sprintf` 里：同一句话有好几个写入点，改一个说法就得逐处找，
// 找漏一处就出现两种说法。现在只有这里认这两个词——别处一律向本文件要半句，不许自己拼
// （`labels_test.go` 有一条扫描判据盯着这件事）。
//
// 文案一个字都没改：这批只是抽构造点，不是改措辞。

// CurrentValue 拼「<说明>，当前值：<值>」——那半句「，当前值：」只在这里成形。
//
// text 是这句话前半截（到逗号为止，不含逗号）；value 是原值的呈现形态
// （要不要加引号由调用方决定：它知道那是路径、数字还是一段配置原文）。
func currentvalue(text, value string) string {
	return text + "，当前值：" + value
}

// ValueInvalid 拼「<项名> 取值非法：<值>」：项名与标签之间**空一格**，配置键就照键名写
// （`upload.small_file`、`execution.timeout_execute`）。
func valueinvalid(name, value string) string {
	return name + " 取值非法：" + value
}

// ValueInvalidTight 与 ValueInvalid 同义，只是项名与标签之间**不空格**——项名是一句中文、
// 不是一个键名时用它（「上传阈值取值非法：…」）。两种版式各一个入口，不给调用方留
// 「自己数一下要不要空格」这种活。
func ValueInvalidTight(subject, value string) string {
	return subject + "取值非法：" + value
}

// ValueInvalidReason 是「取值非法」那句括号里的理由。理由本身也是字，所以归这里；
// 站里只挑一个枚举值，不再自己拼括号与措辞。
type ValueInvalidReason string

const (
	// ReasonNotInt 「（须为整数）」。
	ReasonNotInt ValueInvalidReason = "须为整数"
	// ReasonNotBool 「（只认 true / false）」。
	ReasonNotBool ValueInvalidReason = "只认 true / false"
	// ReasonNotQuoted 「（字符串值要带引号）」。
	ReasonNotQuoted ValueInvalidReason = "字符串值要带引号"
	// ReasonPort 「（须为 1-65535）」。
	ReasonPort ValueInvalidReason = "须为 1-65535"
	// ReasonPositiveSeconds 「（须为正整数，单位秒）」。
	ReasonPositiveSeconds ValueInvalidReason = "须为正整数，单位秒"
	// ReasonPositiveBytes 「（须为正整数，单位字节）」。
	ReasonPositiveBytes ValueInvalidReason = "须为正整数，单位字节"
	// ReasonPositive 「（须为正整数）」。
	ReasonPositive ValueInvalidReason = "须为正整数"
	// ReasonNonNegative 次数类取值：0 有含义（不重试），所以不套「须为正整数」。
	ReasonNonNegative ValueInvalidReason = "须为 0 或正整数"
)

// ValueInvalidBecause 拼「<项名> 取值非法：<值>（<理由>）」——括号是全角，紧贴值。
func ValueInvalidBecause(name, value string, reason ValueInvalidReason) string {
	return valueinvalid(name, value) + "（" + string(reason) + "）"
}

// UploadThresholdOrder 上传阈值那两句的先后：「large_file(9) 不能小于 small_file(10)」。
//
// 括号里的键名与数字一起成形：站里只递两个值，不拼键名。
func UploadThresholdOrder(large, small int) string {
	return fmt.Sprintf("large_file(%d) 不能小于 small_file(%d)", large, small)
}

// tagLabel 是那句话开头那个方括号标签（`[ERROR]` / `[警告]`）唯一的成形处：
// 配色与方括号只在这里写一次，别处把标签名递进来。
func tagLabel(color, label string) string {
	return color + "[" + label + "]" + ansiReset
}
