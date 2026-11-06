package parse

import "strings"

// 违规样本：解析站自己拼星号、自己替换明文——遮的构造点有两个了。

// Scrub 把明文从一段文本里抹掉。
func Scrub(text, secret string) string {
	return strings.ReplaceAll(text, secret, "****")
}
