package mask

import (
	"encoding/csv"
	"strings"
)

// 内联清单（`-f 10.0.0.1,22,root,密码,私钥路径,口令`）的遮法。
//
// 与 Secret 分开：那一条是「显示某个凭据值、让人对上号」（前 2 后 2），这一条是
// 「整张记录照原样显示，只把凭据那几段整个抹掉」——IP、端口、用户名、私钥路径都要留着，
// 使用者靠它们认出是哪台机器。
//
// 按 CSV 字段读、按 CSV 写回：值可能像 Excel 导出那样整段带引号，引号里也可能包着逗号，
// 按裸逗号切会把段号切错、把密码漏在外面。读不成一条记录时原样返回——那是取值判定那头
// 该拒绝的东西，不在这里猜。

// InlineRecord 遮一条内联节点记录的凭据段（第 4 段密码、第 6 段口令）。
func InlineRecord(s string) string {
	fields, ok := singleRecord(s)
	if !ok {
		return s
	}
	for _, idx := range []int{3, 5} {
		if idx < len(fields) && strings.TrimSpace(fields[idx]) != "" {
			fields[idx] = maskToken
		}
	}
	var b strings.Builder
	w := csv.NewWriter(&b)
	if err := w.Write(fields); err != nil {
		return s
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return s
	}
	return strings.TrimSuffix(b.String(), "\n")
}

// InlineRecordIf 是内联清单就遮，否则原样返回（调用方不必自己判形状）。
func InlineRecordIf(s string) string {
	if looksLikeInlineList(s) {
		return InlineRecord(s)
	}
	return s
}

// looksLikeInlineList 是不是「带凭据段的内联清单」：恰好一条记录、首列是 IPv4、
// 且第 4 或第 6 段写了东西。裸 IP 没有凭据可遮，不遮。
func looksLikeInlineList(s string) bool {
	fields, ok := singleRecord(s)
	if !ok || len(fields) == 0 || !isIPv4(strings.TrimSpace(fields[0])) {
		return false
	}
	for _, idx := range []int{3, 5} {
		if idx < len(fields) && strings.TrimSpace(fields[idx]) != "" {
			return true
		}
	}
	return false
}

// singleRecord 把文本读成唯一的一条记录；读不出、或不止一条时返回 false。
func singleRecord(text string) ([]string, bool) {
	r := csv.NewReader(strings.NewReader(strings.TrimPrefix(text, "\ufeff")))
	r.FieldsPerRecord = -1
	r.Comment = '#'
	rec, err := r.Read()
	if err != nil {
		return nil, false
	}
	if _, err := r.Read(); err == nil {
		return nil, false // 不止一条：那不是一个内联值
	}
	return rec, true
}

// isIPv4 严格认 IPv4 字面量。
//
// 与解析站那一份**各写一遍、不合并**（上一张图 04 号工单的 Answer：解析认清单、
// mask 认数据行，两处各写一份）。合成一份就要让横切包依赖站，方向就反了。
func isIPv4(s string) bool {
	parts := strings.Split(s, ".")
	if len(parts) != 4 {
		return false
	}
	for _, p := range parts {
		if p == "" || len(p) > 3 {
			return false
		}
		n := 0
		for _, r := range p {
			if r < '0' || r > '9' {
				return false
			}
			n = n*10 + int(r-'0')
		}
		if n > 255 {
			return false
		}
	}
	return true
}
