package mask

import (
	"strings"

	"sshfleet2/internal/criterion"
)

// 内联清单（`-f 10.0.0.1,22,root,密码,私钥路径,口令`）的遮法。
//
// 与 `Secret` 同一形态——都是「前 2 + `****` + 后 2」，四个字符以内整段抹。区别只在这条要
// 按 CSV 读、只遮凭据那两段：IP、端口、用户名、私钥路径都要留着，使用者靠它们认出是哪台机器。
//
// 按 CSV 字段读、按 CSV 写回：值可能像 Excel 导出那样整段带引号，引号里也可能包着逗号，
// 按裸逗号切会把段号切错、把密码漏在外面。读不成一条记录时原样返回——那是取值判定那头
// 该拒绝的东西，不在这里猜。

// InlineRecord 遮一条内联节点记录的凭据段（第 4 段密码、第 6 段口令）。
//
// 遮出来与 `Secret` 同一个样子：整个工程只有一种遮法（建站 07 规格第六节）。
func InlineRecord(s string) string {
	fields, ok := singleRecord(s)
	if !ok {
		return s
	}
	maskCredentialFields(fields)
	out, err := joinCSV(fields)
	if err != nil {
		return s
	}
	return out
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
	if !ok || len(fields) == 0 || !criterion.IsIPv4(fields[0]) {
		return false
	}
	for _, idx := range credentialColumns {
		if idx < len(fields) && strings.TrimSpace(fields[idx]) != "" {
			return true
		}
	}
	return false
}

// singleRecord 把文本读成唯一的一条记录；读不出、或不止一条时返回 false。
func singleRecord(text string) ([]string, bool) {
	r := newCSVReader(text, true)
	rec, err := r.Read()
	if err != nil {
		return nil, false
	}
	if _, err := r.Read(); err == nil {
		return nil, false // 不止一条：那不是一个内联值
	}
	return rec, true
}
