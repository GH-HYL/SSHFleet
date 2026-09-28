// CSV 取值：一个参数的值既可以是 CSV 文件路径，也可以是同样格式的内联文本。
//
// 全文只有这一处解析实现——`-f`（节点清单）与 `-a`（代填）共用。两者的列语义
// 不同（清单是 IP/端口/账号…，代填是内容/触发词…），但「取值 → 逐行解析」这条
// 链路必须逐字一致：`#` 注释行、空行、BOM、双引号、变长列的容忍口径，一处定、两处用。
//
// 内联值与文件内容唯一的不同在装载阶段：命令行里传不进真实换行，内联值先经
// ExpandEscapedNewlines 把字面 `\n` 补成真换行。补完之后两者走同一个
// ReadCSVRows，解析上没有任何分叉——内联不是另一套格式。
package common

import (
	"bytes"
	"encoding/csv"
	"io"
	"strings"
)

// ExpandEscapedNewlines 把字面 `\n`（反斜杠 + n）换成真实换行。
//
// 只给内联值用：命令行要写多行只能这么写。文件内容是原样读进来的，
// `\n` 在文件里就是两个普通字符，不要对它调这个。
func ExpandEscapedNewlines(text string) string {
	return strings.ReplaceAll(text, `\n`, "\n")
}

// IsBinaryContent 判断内容算不算二进制：出现 NUL 字节即算。
// 只看前 1KB——判定用不着读完整份。
func IsBinaryContent(data []byte) bool {
	if len(data) > 1024 {
		data = data[:1024]
	}
	return bytes.IndexByte(data, 0) >= 0
}

// CSVRow 一条记录。Line 是它在原文里的行号（1 基）：被跳过的空行与注释行照样占
// 行号，所以报出来的行号就是文本里的行号，直接数得到。
type CSVRow struct {
	Line   int
	Fields []string
}

// ReadCSVRows 把 CSV 文本读成记录。
//
// 口径（清单与代填共用同一份，不因来源是文件还是内联而变）：
//   - 剥开头的 UTF-8 BOM（记事本存 CSV 默认带它，留着会让首行的 `#` 注释失效）
//   - `#` 在行首的整行跳过
//   - 空行跳过
//   - 双引号成对时，引号里可以包住逗号与换行
//   - 每行列数不限（FieldsPerRecord = -1），缺的列由调用方按自己的口径补
//
// 只有空白的行不是空行，照常读成一条记录（首列是空白），由调用方的列校验去报。
// 双引号不配对这类语法错原样返回 *csv.ParseError（自带行号），文案由调用方写。
func ReadCSVRows(text string) ([]CSVRow, error) {
	src := strings.TrimPrefix(text, "\ufeff")
	r := csv.NewReader(strings.NewReader(src))
	r.FieldsPerRecord = -1
	r.Comment = '#'

	var (
		rows []CSVRow
		line = 1 // 光标当前所在的行号
		prev = 0
	)
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		off := int(r.InputOffset())
		chunk := src[prev:off] // 这次读消耗掉的原文：开头的空行/注释行属于它，记录本身也在里面
		rows = append(rows, CSVRow{Line: recordStartLine(chunk, line), Fields: rec})
		line += strings.Count(chunk, "\n")
		prev = off
	}
	return rows, nil
}

// recordStartLine 记录在原文里的起始行号。解析器会把开头的空行与注释行一起吃掉，
// 所以要从 chunk 里数过去：跳掉几行，记录就落在第几行。
func recordStartLine(chunk string, line int) int {
	for chunk != "" {
		text, rest, found := strings.Cut(chunk, "\n")
		text = strings.TrimRight(text, "\r")
		if text != "" && !strings.HasPrefix(text, "#") {
			return line
		}
		line++
		if !found {
			break
		}
		chunk = rest
	}
	return line
}
