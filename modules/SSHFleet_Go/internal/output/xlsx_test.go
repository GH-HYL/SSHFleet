package output

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"

	"sshfleet/internal/batch"
	"sshfleet/internal/config"
	"sshfleet/internal/ssh"
)

// xlsx 格式化回归（对位旧 xlsx.py）：格式错在表格里肉眼难查，逐条断言。

func testCfg() *config.Config {
	cfg := &config.Config{}
	cfg.Paths.OutputXlsx = "output.xlsx"
	cfg.Paths.ResultsXlsx = "results.xlsx"
	return cfg
}

func testResults() *batch.Results {
	ok := 0
	return &batch.Results{Items: []ssh.Result{
		{
			Seq: 1, IP: "10.0.0.1", Port: 22, User: "root",
			ConnectSuccess: true, ExitCode: &ok,
			ConnectCostTime: 0.123, ExecCostTime: 0.456,
			Output: "line one\nline two\n",
		},
		{
			Seq: 2, IP: "10.0.0.2", Port: 22, User: "root",
			ConnectSuccess: false, ConnectCostTime: 0.5,
			Error: strPtr("dial tcp 10.0.0.2:22: connect: connection refused"),
		},
	}}
}

func categoryOf(r ssh.Result) string {
	if r.ConnectSuccess {
		return "执行成功"
	}
	return "拒绝网络连接"
}

func openXlsx(t *testing.T, path string) *excelize.File {
	t.Helper()
	f, err := excelize.OpenFile(path)
	if err != nil {
		t.Fatalf("打开 %s 失败：%v", path, err)
	}
	t.Cleanup(func() { _ = f.Close() })
	return f
}

func cellOf(t *testing.T, f *excelize.File, sheet, cell string) string {
	t.Helper()
	v, err := f.GetCellValue(sheet, cell)
	if err != nil {
		t.Fatalf("读取 %s!%s 失败：%v", sheet, cell, err)
	}
	return v
}

func TestOutputXlsxLayout(t *testing.T) {
	dir := t.TempDir()
	if err := WriteOutputXlsx(dir, testResults(), testCfg(), "execute", nil, categoryOf); err != nil {
		t.Fatal(err)
	}
	f := openXlsx(t, filepath.Join(dir, "output.xlsx"))
	sheet := f.GetSheetName(0)
	if sheet != "执行日志" {
		t.Fatalf("工作表名应为「执行日志」，实际 %q", sheet)
	}

	want := [][]string{
		{"IP地址", "事件类型", "内容详情"},
		{"10.0.0.1", "连接: 成功 - 0.123s", ""},
		{"10.0.0.1", "执行: 成功 - 0.456s", ""},
		{"10.0.0.1", "标准输出和错误输出", "line one"},
		{"10.0.0.1", "标准输出和错误输出", "line two"},
		{"10.0.0.1", "分类: 执行成功", ""},
		{"", "", ""}, // 分隔行（只有填充色）
		{"10.0.0.2", "连接: 失败 - 0.500s", ""},
		{"10.0.0.2", "执行: 失败 - 0.000s", ""},
		{"10.0.0.2", "分类: 拒绝网络连接", ""},
		{"", "", ""},
	}
	rows, err := f.GetRows(sheet)
	if err != nil {
		t.Fatal(err)
	}
	// GetRows 不含「只有填充色、无内容」的末行分隔行，故按前 len(want)-1 行比对
	if len(rows) != len(want)-1 {
		t.Fatalf("应有 %d 行内容，实际 %d 行：%v", len(want)-1, len(rows), rows)
	}
	for r := range rows {
		for c := range want[r] {
			cell, _ := excelize.CoordinatesToCellName(c+1, r+1)
			if got := cellOf(t, f, sheet, cell); got != want[r][c] {
				t.Fatalf("%s 应为 %q，实际 %q", cell, want[r][c], got)
			}
		}
	}

	// 列宽与冻结
	for _, c := range []struct {
		col  string
		want float64
	}{{"A", 15}, {"B", 20}, {"C", 60}} {
		if got, err := f.GetColWidth(sheet, c.col); err != nil || got != c.want {
			t.Fatalf("%s 列宽应为 %v，实际 %v（err=%v）", c.col, c.want, got, err)
		}
	}
	panes, err := f.GetPanes(sheet)
	if err != nil {
		t.Fatal(err)
	}
	if !panes.Freeze || panes.TopLeftCell != "A2" {
		t.Fatalf("应冻结首行（A2），实际 freeze=%v topLeft=%q", panes.Freeze, panes.TopLeftCell)
	}

	// 分隔行必须是浅蓝填充（D9E1F2）：每条结果之后各一行，含最后一行
	for _, row := range []int{7, 11} {
		cell := fmt.Sprintf("A%d", row)
		styleID, err := f.GetCellStyle(sheet, cell)
		if err != nil {
			t.Fatal(err)
		}
		style, err := f.GetStyle(styleID)
		if err != nil {
			t.Fatal(err)
		}
		if len(style.Fill.Color) == 0 || !strings.EqualFold(style.Fill.Color[0], "D9E1F2") {
			t.Fatalf("%s 分隔行应为 D9E1F2 填充，实际 %+v", cell, style.Fill)
		}
	}
}

func TestResultsXlsxLayoutAndAutoWidth(t *testing.T) {
	dir := t.TempDir()
	if err := WriteResultsXlsx(dir, testResults(), testCfg(), "execute", nil, categoryOf); err != nil {
		t.Fatal(err)
	}
	f := openXlsx(t, filepath.Join(dir, "results.xlsx"))
	sheet := f.GetSheetName(0)
	if sheet != "Results" {
		t.Fatalf("工作表名应为 Results，实际 %q", sheet)
	}
	if got := cellOf(t, f, sheet, "A1"); got != "seq" {
		t.Fatalf("表头首格应为 seq，实际 %q", got)
	}
	if got := cellOf(t, f, sheet, "M2"); got != "执行成功" {
		t.Fatalf("M 列应为分类，实际 %q", got)
	}

	// A–M 自适应：宽度 >= 表头/内容宽度；N、O 固定
	headers := []string{"seq", "ip", "port", "user", "connect_success", "exit_code", "connect_cost_time",
		"exec_cost_time", "total_bytes", "total_files", "success_files", "failed_files", "分类", "error", "output"}
	values := [][]string{
		{"1", "10.0.0.1", "22", "root", "TRUE", "0", "0.123", "0.456", "", "", "", "", "执行成功", "", "line one\nline two"},
		{"2", "10.0.0.2", "22", "root", "FALSE", "", "0.5", "0", "", "", "", "", "拒绝网络连接", "dial tcp 10.0.0.2:22: connect: connection refused", ""},
	}
	for i := range headers {
		col := colName(i + 1)
		got, err := f.GetColWidth(sheet, col)
		if err != nil {
			t.Fatal(err)
		}
		if i >= resultsFixedWidthColumns {
			if got != resultsFixedWidth {
				t.Fatalf("%s 列（N/O）应固定 %v 宽，实际 %v", col, resultsFixedWidth, got)
			}
			continue
		}
		want := DisplayWidth(headers[i])
		for _, row := range values {
			if w := DisplayWidth(row[i]); w > want {
				want = w
			}
		}
		want += 2
		if int(got) != want {
			t.Fatalf("%s 列应为最合适列宽 %d，实际 %v", col, want, got)
		}
	}

	// 数据单元格要有细边框（对位旧 Border(thin)）
	styleID, err := f.GetCellStyle(sheet, "B2")
	if err != nil {
		t.Fatal(err)
	}
	style, err := f.GetStyle(styleID)
	if err != nil {
		t.Fatal(err)
	}
	if len(style.Border) == 0 {
		t.Fatal("数据单元格应带细边框，实际无边框样式")
	}
}

func TestCleanForExcel(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"去 ANSI 颜色", "\x1b[31merror\x1b[0m ok", "error ok"},
		{"去控制字符", "a\x00b\x07c", "abc"},
		{"去零宽字符", "a\u200bb\ufeffc", "abc"},
		{"统一换行", "a\r\nb\rc", "a\nb\nc"},
		{"保留制表符", "a\tb", "a\tb"},
		{"行首等号防公式", "=SUM(A1:A2)", " =SUM(A1:A2)"},
		{"行首等号带缩进", "  =cmd", "   =cmd"},
		{"非行首等号不动", "a=b", "a=b"},
		{"去首尾空行", "\n\na\n\n", "a"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := cleanForExcel(c.in); got != c.want {
				t.Fatalf("cleanForExcel(%q) 应为 %q，实际 %q", c.in, c.want, got)
			}
		})
	}
}
