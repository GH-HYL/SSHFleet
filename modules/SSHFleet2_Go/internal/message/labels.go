package message

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
func CurrentValue(text, value string) string {
	return text + "，当前值：" + value
}

// ValueInvalid 拼「<项名> 取值非法：<值>」：项名与标签之间**空一格**，配置键就照键名写
// （`upload.small_file`、`execution.timeout_execute`）。
func ValueInvalid(name, value string) string {
	return name + " 取值非法：" + value
}

// ValueInvalidTight 与 ValueInvalid 同义，只是项名与标签之间**不空格**——项名是一句中文、
// 不是一个键名时用它（「上传阈值取值非法：…」）。两种版式各一个入口，不给调用方留
// 「自己数一下要不要空格」这种活。
func ValueInvalidTight(subject, value string) string {
	return subject + "取值非法：" + value
}
