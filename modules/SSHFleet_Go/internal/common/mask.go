// 凭据脱敏：明文密码 / 口令不得原样落进任何日志、报告或归档。
package common

import "strings"

// MaskSecret 显示一个凭据值：长度 > 4 位时显示「前 2 位 + **** + 后 2 位」，
// ≤ 4 位时整个显示为 ****。
//
// 中间的 * 固定 4 个、不随真实长度变化——连长度也不泄露。
func MaskSecret(s string) string {
	r := []rune(s)
	if len(r) <= 4 {
		return "****"
	}
	return string(r[:2]) + "****" + string(r[len(r)-2:])
}

// LooksLikeInlineList 判断一个 -f 取值是不是内联清单（首段是 IPv4 且含逗号）。
// 文件路径形式的清单不含逗号，不会被误判。
func LooksLikeInlineList(s string) bool {
	if !strings.Contains(s, ",") {
		return false
	}
	return looksLikeIPv4(strings.TrimSpace(strings.SplitN(s, ",", 2)[0]))
}

// looksLikeIPv4 只做形状判断（四段点分十进制），不校验取值范围——
// 这里只用来决定"要不要脱敏"，不是节点校验。
func looksLikeIPv4(s string) bool {
	parts := strings.Split(s, ".")
	if len(parts) != 4 {
		return false
	}
	for _, p := range parts {
		if p == "" || len(p) > 3 {
			return false
		}
		for _, c := range p {
			if c < '0' || c > '9' {
				return false
			}
		}
	}
	return true
}

// MaskInlineList 内联清单（IP,端口,用户名,密码,私钥路径,口令）的脱敏：
// **只把凭据那几段整个换成 ******（第 4 段密码、第 6 段口令），
// IP / 端口 / 用户名 / 私钥路径原样保留——用户要靠这几段认出是哪台机器。
//
// 这里不比照 MaskSecret 的"前 2 后 2"：那条规格是给"显示某个凭据值、让人对上号"用的。
func MaskInlineList(s string) string {
	parts := strings.Split(s, ",")
	for _, idx := range []int{3, 5} {
		if idx < len(parts) && strings.TrimSpace(parts[idx]) != "" {
			parts[idx] = "****"
		}
	}
	return strings.Join(parts, ",")
}

// MaskInlineListIf 是内联清单就脱敏，否则原样返回（调用方不必自己判形状）。
func MaskInlineListIf(s string) string {
	if LooksLikeInlineList(s) {
		return MaskInlineList(s)
	}
	return s
}

// MaskCommandLine 把命令行里的内联清单脱敏（逐个 argv 判断，含 `-f=IP,...` 这种写法）。
func MaskCommandLine(argv []string) []string {
	out := make([]string, len(argv))
	for i, a := range argv {
		switch {
		case LooksLikeInlineList(a):
			out[i] = MaskInlineList(a)
		case strings.HasPrefix(a, "-f=") && LooksLikeInlineList(a[3:]):
			out[i] = "-f=" + MaskInlineList(a[3:])
		default:
			out[i] = a
		}
	}
	return out
}
