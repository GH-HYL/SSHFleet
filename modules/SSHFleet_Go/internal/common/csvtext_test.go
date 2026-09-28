package common

import (
	"encoding/csv"
	"errors"
	"strings"
	"testing"
)

// 取值 → 记录的解析口径：注释行、空行、BOM、引号、变长列。
// 这份口径是 -f（清单）与 -a（代填）共用的全部依据。

func TestReadCSVRowsBasic(t *testing.T) {
	rows, err := ReadCSVRows("1.1.1.1,22,root,pw\n2.2.2.2\n")
	if err != nil {
		t.Fatalf("应解析通过：%v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("应解析出 2 行，实际 %d 行", len(rows))
	}
	if rows[0].Fields[0] != "1.1.1.1" || rows[0].Line != 1 {
		t.Fatalf("第 1 行不对：%+v", rows[0])
	}
	// 变长行容忍：第二行只有一列，不缺列不报错
	if len(rows[1].Fields) != 1 || rows[1].Line != 2 {
		t.Fatalf("第 2 行不对：%+v", rows[1])
	}
}

// 行号是文本里的行号：被跳过的注释行与空行照样占行号。
// 只有空白的行不是空行——它照常读成一条记录（首列是空白），由调用方的列校验去报。
func TestReadCSVRowsLineNumbers(t *testing.T) {
	rows, err := ReadCSVRows("# 注释\n\n1,甲\n   \n2,乙\n")
	if err != nil {
		t.Fatalf("应解析通过：%v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("注释行与空行跳过、纯空白行保留，应剩 3 行，实际 %d 行：%+v", len(rows), rows)
	}
	if rows[0].Line != 3 || rows[1].Line != 4 || rows[2].Line != 5 {
		t.Fatalf("行号应按原文数（3、4、5），实际 %d、%d、%d", rows[0].Line, rows[1].Line, rows[2].Line)
	}
	if rows[1].Fields[0] != "   " {
		t.Fatalf("纯空白行应原样保留首列：%q", rows[1].Fields[0])
	}
}

// 内联值的字面 \n 是行分隔符：命令行里写不出真实换行。
func TestExpandEscapedNewlines(t *testing.T) {
	rows, err := ReadCSVRows(ExpandEscapedNewlines(`1,请选择架构\ndeb,请选择包格式`))
	if err != nil {
		t.Fatalf("应解析通过：%v", err)
	}
	if len(rows) != 2 || rows[0].Fields[0] != "1" || rows[1].Fields[1] != "请选择包格式" {
		t.Fatalf("字面 \\n 应分成两行：%+v", rows)
	}
}

// 双引号是语法的：引号里的逗号与换行都归同一个字段。
func TestReadCSVRowsQuoted(t *testing.T) {
	rows, err := ReadCSVRows(`"甲,乙","丙""丁"` + "\n" + `"跨` + "\n" + `行",尾`)
	if err != nil {
		t.Fatalf("应解析通过：%v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("应解析出 2 行，实际 %d 行：%+v", len(rows), rows)
	}
	if rows[0].Fields[0] != "甲,乙" || rows[0].Fields[1] != `丙"丁` {
		t.Fatalf("引号内的逗号与转义引号应归同一字段：%+v", rows[0].Fields)
	}
	if rows[1].Fields[0] != "跨\n行" {
		t.Fatalf("引号内的换行应归同一字段：%q", rows[1].Fields[0])
	}
}

// 引号不配对：报语法错，且带上行号（文案由调用方写）。
func TestReadCSVRowsBadQuotes(t *testing.T) {
	_, err := ReadCSVRows("1,甲\n2,\"乙\n")
	if err == nil {
		t.Fatal("引号不配对应报错")
	}
	var parseErr *csv.ParseError
	if !errors.As(err, &parseErr) || parseErr.Line != 2 {
		t.Fatalf("应报第 2 行的解析错，实际：%v", err)
	}
}

// BOM 只在文本开头，剥掉之后首行的 # 才认得出来。
func TestReadCSVRowsBOM(t *testing.T) {
	rows, err := ReadCSVRows("\ufeff# 注释\n1,甲\n")
	if err != nil {
		t.Fatalf("应解析通过：%v", err)
	}
	if len(rows) != 1 || rows[0].Fields[0] != "1" || rows[0].Line != 2 {
		t.Fatalf("BOM 应被剥掉、注释行应跳过：%+v", rows)
	}
}

// 空文本不是错误：有没有内容由调用方按自己的口径判。
func TestReadCSVRowsEmptyText(t *testing.T) {
	rows, err := ReadCSVRows("# 只有注释\n\n")
	if err != nil {
		t.Fatalf("空文本不该报错：%v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("应没有记录，实际：%+v", rows)
	}
	rows, err = ReadCSVRows(ExpandEscapedNewlines(`\n\n`))
	if err != nil || len(rows) != 0 {
		t.Fatalf("全空行应没有记录，实际：%+v（%v）", rows, err)
	}
}

func TestIsBinaryContent(t *testing.T) {
	if IsBinaryContent([]byte("1.1.1.1,22,root,pw\n")) {
		t.Fatal("纯文本不该判成二进制")
	}
	if !IsBinaryContent([]byte("PK\x03\x04\x00\x00")) {
		t.Fatal("出现 NUL 应判成二进制")
	}
	// 只看前 1KB：NUL 落在 1KB 之后不算
	long := strings.Repeat("a", 2048) + "\x00"
	if IsBinaryContent([]byte(long)) {
		t.Fatal("1KB 之后的 NUL 不该判成二进制")
	}
}
