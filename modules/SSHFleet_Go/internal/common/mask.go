// 凭据脱敏：明文密码 / 口令不得原样落进任何日志、报告或归档。
package common

import (
	"encoding/csv"
	"strings"
)

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

// LooksLikeInlineList 判断一个 -f 取值是不是带凭据段的内联清单：
// 一条内联节点记录，且写了换行以外的凭据段（第 4 段密码、第 6 段口令）。
//
// 记录像不像由 LooksLikeInlineNodeRecord 一处判——取值判定与这里共用同一把尺子，
// 不会出现「放行认得出、脱敏认不出」（ADR-0023 决定 5）。裸 IP 没有凭据可脱，
// 文件路径也读不出节点，两者都返回 false。
func LooksLikeInlineList(s string) bool {
	if !LooksLikeInlineNodeRecord(s) {
		return false
	}
	return strings.Contains(s, ",")
}

// MaskInlineList 内联清单（IP,端口,用户名,密码,私钥路径,口令）的脱敏：
// **只把凭据那几段整个换成 ******（第 4 段密码、第 6 段口令），
// IP / 端口 / 用户名 / 私钥路径原样保留——用户要靠这几段认出是哪台机器。
//
// 按 CSV 字段读、按 CSV 写回：值可以像 Excel 导出那样整段带引号（`"10.0.0.1","22",…`），
// 引号里也可能包着逗号，按裸逗号切会把段号切错、把密码漏在外面。
// 读不成一条记录时原样返回——那是取值判定那头该拒绝的东西，不在这里猜。
//
// 这里不比照 MaskSecret 的"前 2 后 2"：那条规格是给"显示某个凭据值、让人对上号"用的。
func MaskInlineList(s string) string {
	record, ok := csvRecord(s)
	if !ok {
		return s
	}
	for _, idx := range []int{3, 5} {
		if idx < len(record.Fields) && strings.TrimSpace(record.Fields[idx]) != "" {
			record.Fields[idx] = "****"
		}
	}
	var b strings.Builder
	w := csv.NewWriter(&b)
	if err := w.Write(record.Fields); err != nil {
		return s
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return s
	}
	return strings.TrimSuffix(b.String(), "\n")
}

// csvRecord 把文本读成唯一的一条记录；读不出、或不止一条时返回 false。
func csvRecord(text string) (*CSVRow, bool) {
	rows, err := ReadCSVRows(text)
	if err != nil || len(rows) != 1 {
		return nil, false
	}
	return &rows[0], true
}

// MaskInlineListIf 是内联清单就脱敏，否则原样返回（调用方不必自己判形状）。
func MaskInlineListIf(s string) string {
	if LooksLikeInlineList(s) {
		return MaskInlineList(s)
	}
	return s
}

// MaskCommandLine 把命令行里带凭据的取值脱敏：内联清单（含 `-f=IP,...` 写法），
// 以及 `--change-password` 的值（含 `--change-password=密码` 写法——明文密文都脱）。
//
// 这里按**旗标名**认，不靠形状猜：新密码是明确知道身份的凭据，形态上没有特征可认。
func MaskCommandLine(argv []string) []string {
	out := make([]string, len(argv))
	maskNext := false
	for i, a := range argv {
		switch {
		case maskNext:
			out[i] = MaskSecret(a)
			maskNext = false
		case a == "--change-password":
			out[i] = a
			maskNext = true
		case strings.HasPrefix(a, "--change-password="):
			out[i] = "--change-password=" + MaskSecret(a[len("--change-password="):])
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
