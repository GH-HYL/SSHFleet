package message

import (
	"fmt"
	"strconv"
	"strings"

	"sshfleet2/internal/contract"
)

// 解析站路上那几句「说一声就好、不必中断」的提示。
//
// 纸上装的是种类与事实（`contract.Notice`），句子在这里成形——解析站自己不写终端，
// 也不许自己拼一句字塞过去（它拼不出来：`Line` 的字段不导出）。

// NoticeLine 把纸上那条提示变成一句话。塔尖照 Kind 要，它自己不认识任何一种提示。
func NoticeLine(n contract.Notice) Line {
	switch n.Kind {
	case contract.NoticeManifestHeader:
		return ManifestHeaderDropped()
	case contract.NoticeUnusedKey:
		return UnusedKeyNotice(n.Rows)
	}
	return Line{}
}

// UnusedKeyNotice 是「清单里配了私钥、这次没加 -k」的那句提示。
//
// 只说前五行、多于一行的说「共 N 行」——清单可能几百行，一句提示刷满屏没有意义。
// 逐字照老版，不带颜色（老版这一句就是纯文本，没套 [INFO]）。
func UnusedKeyNotice(rows []int) Line {
	if len(rows) == 0 {
		return Line{}
	}
	const shown = 5
	parts := make([]string, 0, shown)
	for i, r := range rows {
		if i >= shown {
			break
		}
		parts = append(parts, strconv.Itoa(r))
	}
	list := strings.Join(parts, "、")
	tail := "这几台会用密码登录"
	switch {
	case len(rows) > shown:
		list = fmt.Sprintf("共 %d 行配了私钥（如第 %s 行）", len(rows), list)
	case len(rows) == 1:
		list = "第 " + list + " 行配了私钥"
		tail = "这一台会用密码登录"
	default:
		list = "第 " + list + " 行配了私钥"
	}
	return Line{s: fmt.Sprintf("[提示] 清单%s，本次没加 -k，%s\n", list, tail)}
}
