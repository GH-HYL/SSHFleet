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
		list = keyRows(list)
		tail = "这一台会用密码登录"
	default:
		list = keyRows(list)
	}
	return Line{s: fmt.Sprintf("[提示] 清单%s，本次没加 -k，%s\n", list, tail)}
}

// keyRows 是「第 X 行配了私钥」那半句：单行与多行两个分支只差抬头，句子本身一处成形。
func keyRows(list string) string {
	return "第 " + list + " 行配了私钥"
}

// UploadSkippedLine 与 `UploadSkippedText` 同一句话，给屏幕用（交互路径上老版在
// 「开始执行」之后补这一行）。
func UploadSkippedLine(names []string) Line {
	return Line{s: UploadSkippedText(names) + "\n"}
}

// UploadSkippedText 是「上传源里有链接被过滤」那一句，进执行期日志（老版记在开头两行之前）。
//
// 名单最多列五个，多的写「等 N 个」——清单可能几百个，一句提示刷满日志没有意义
// （与解析站 collect.summarize 同一条规矩，两处各拼一遍是因为一处给屏幕、一处给日志）。
func UploadSkippedText(names []string) string {
	const shown = 5
	parts := make([]string, 0, shown)
	for i, n := range names {
		if i >= shown {
			break
		}
		parts = append(parts, n)
	}
	list := strings.Join(parts, "、")
	if len(names) > shown {
		list += fmt.Sprintf(" 等 %d 个", len(names))
	}
	return fmt.Sprintf("[提示] 上传源中有 %d 个软链接/快捷方式被过滤（不上传）：%s", len(names), list)
}
