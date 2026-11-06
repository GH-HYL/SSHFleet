package report

// 两张表格：跑完之后落在本次目录里的 xlsx。
//
// 版式照老版（`output/xlsx.go`）一格不改——使用者照着这两个文件查问题，列序、表头、颜色、
// 列宽、筛选都别重新设计。**字不在这里成形**：每一行的三列由 `message` 摆（见
// `message.ResultRows`），本文件只负责往格子里写、加样式、量列宽。

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/xuri/excelize/v2"

	"sshfleet2/internal/contract"
	"sshfleet2/internal/message"
)

// 表格清理：写进 XML 之前把任意字节流整成能装的东西。
var (
	// ansiEscape 是颜色与光标那类转义序列（CSI / OSC / 其余 ESC 段）。
	ansiEscape = regexp.MustCompile(
		`\x1b\[[0-9;]*[a-zA-Z]` +
			`|\x1b\][^\x07]*(?:\x07|\x1b\\)` +
			`|\x1b[@-_][0-?]*[-/]*[@-~]`)
	// excelInvisible 是非打印与零宽字符（保留制表与换行）。C1 段必须写码点
	// `\u007f-\u009f`——写成原始字节不是合法 UTF-8，正则编译会直接失败。
	excelInvisible = regexp.MustCompile(
		"[\x00-\x08\x0b\x0c\x0e-\x1f\u007f-\u009f" +
			"\u200b-\u200f\u2028-\u202f\u2060-\u206f\ufeff]")
)

// cleanForExcel 把一段原文整成能写进 xlsx 的样子：
// 非法 UTF-8 替换 → 去 ANSI 转义 → 去非打印与零宽字符 → 统一换行 → 行首 `=` 前补空格
// （防被判成公式）→ 去掉首尾空行。
//
// 为什么非清不可：远端输出是任意字节流（颜色、进度条控制符、二进制片段），终端与
// `terminal-output.txt` 吃得下，xlsx 是 XML——控制字符与非法 UTF-8 会把文件写坏。
// 清理是全量唯一入口：任何字符串从这一处过，别在别处再清一遍。
func cleanForExcel(s string) string {
	s = strings.ToValidUTF8(s, "\uFFFD")
	s = ansiEscape.ReplaceAllString(s, "")
	s = excelInvisible.ReplaceAllString(s, "")
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")

	lines := strings.Split(s, "\n")
	for i, ln := range lines {
		trimmed := strings.TrimLeft(ln, " \t")
		if strings.HasPrefix(trimmed, "=") {
			lines[i] = ln[:len(ln)-len(trimmed)] + " " + trimmed
		}
	}
	return strings.Trim(strings.Join(lines, "\n"), "\n")
}

// cell 是写单元格的唯一入口：字符串一律先过 cleanForExcel，数字与布尔原样写。
func cell(f *excelize.File, sheet, at string, v any) error {
	if s, ok := v.(string); ok {
		v = cleanForExcel(s)
	}
	return f.SetCellValue(sheet, at, v)
}

// portNumber 端口那一格按数字写：老版那张表里它就是个数，写成文本会被 Excel 按字符串
// 排序（10022 排到 22 前面）并改成左对齐。端口在纸上由解析站整成十进制串，这里只还原类型。
//
// 解不出来就是 0——老版那一格是 `int`，读不到也是 0。
func portNumber(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}

// sheetStyles 一张表的三个样式：表头、输出明细格、分隔行。
type sheetStyles struct {
	header    int
	detail    int
	separator int
	body      int
}

// outputStyles 是 `terminal-output.xlsx` 的三个样式：表头深蓝底白字加粗居中；输出明细
// 靠上、不折行；结果之间一条浅蓝分隔行。
func outputStyles(f *excelize.File) (sheetStyles, error) {
	var st sheetStyles
	var err error
	if st.header, err = f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true, Color: "FFFFFF", Size: 12},
		Fill:      excelize.Fill{Type: "pattern", Color: []string{"366092"}, Pattern: 1},
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center"},
	}); err != nil {
		return st, err
	}
	if st.detail, err = f.NewStyle(&excelize.Style{
		Alignment: &excelize.Alignment{Vertical: "top"},
	}); err != nil {
		return st, err
	}
	if st.separator, err = f.NewStyle(&excelize.Style{
		Fill: excelize.Fill{Type: "pattern", Color: []string{"D9E1F2"}, Pattern: 1},
	}); err != nil {
		return st, err
	}
	return st, nil
}

// resultsStyles 是 `results.xlsx` 的两个样式：表头蓝底白字带细边框，数据格带细边框。
func resultsStyles(f *excelize.File) (sheetStyles, error) {
	thin := []excelize.Border{
		{Type: "left", Color: "000000", Style: 1},
		{Type: "right", Color: "000000", Style: 1},
		{Type: "top", Color: "000000", Style: 1},
		{Type: "bottom", Color: "000000", Style: 1},
	}
	var st sheetStyles
	var err error
	if st.header, err = f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true, Color: "FFFFFF"},
		Fill:      excelize.Fill{Type: "pattern", Color: []string{"4472C4"}, Pattern: 1},
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center"},
		Border:    thin,
	}); err != nil {
		return st, err
	}
	if st.body, err = f.NewStyle(&excelize.Style{Border: thin}); err != nil {
		return st, err
	}
	return st, nil
}

// writeTerminalXlsx 写 `terminal-output.xlsx`：三列（IP地址、事件类型、内容详情）。
//
// 一台结果摊成几行全由 `message.ResultRows` 决定；这里只管落格与样式。
func writeTerminalXlsx(path string, l contract.Ledger, mode contract.Mode) error {
	if len(l.Items) == 0 {
		return nil
	}

	f := excelize.NewFile()
	defer func() { _ = f.Close() }()
	const sheet = "执行日志"
	if err := f.SetSheetName("Sheet1", sheet); err != nil {
		return err
	}
	st, err := outputStyles(f)
	if err != nil {
		return err
	}

	for i, h := range message.TerminalHeaders() {
		at, _ := excelize.CoordinatesToCellName(i+1, 1)
		if err := cell(f, sheet, at, h); err != nil {
			return err
		}
		if err := f.SetCellStyle(sheet, at, at, st.header); err != nil {
			return err
		}
	}

	row := 1
	for _, item := range l.Items {
		for _, r := range message.ResultRows(item, mode) {
			row++
			if r.Separator {
				first, _ := excelize.CoordinatesToCellName(1, row)
				last, _ := excelize.CoordinatesToCellName(len(message.TerminalHeaders()), row)
				if err := f.SetCellStyle(sheet, first, last, st.separator); err != nil {
					return err
				}
				continue
			}
			for i, v := range []string{item.IP, r.Event, r.Detail} {
				at, _ := excelize.CoordinatesToCellName(i+1, row)
				if err := cell(f, sheet, at, v); err != nil {
					return err
				}
				if r.Output && i == 2 {
					if err := f.SetCellStyle(sheet, at, at, st.detail); err != nil {
						return err
					}
				}
			}
		}
	}

	for i, width := range []float64{15, 20, 120} {
		if err := f.SetColWidth(sheet, colName(i+1), colName(i+1), width); err != nil {
			return err
		}
	}
	last, _ := excelize.CoordinatesToCellName(len(message.TerminalHeaders()), row)
	if err := f.AutoFilter(sheet, "A1:"+last, nil); err != nil {
		return err
	}
	// 冻结首行
	if err := f.SetPanes(sheet, &excelize.Panes{
		Freeze: true, YSplit: 1, TopLeftCell: "A2", ActivePane: "bottomLeft",
	}); err != nil {
		return err
	}
	return f.SaveAs(path)
}

// 两列自由文本的固定列宽：不参与自适应——按内容撑宽会被最长的错误串拉出几百字符。
const (
	resultsErrorWidth  = 30.0
	resultsOutputWidth = 50.0
	// resultsAutoColumns 是参与自适应的列数（A–M），其后两列固定宽。
	resultsAutoColumns = 13
)

// writeResultsXlsx 写 `results.xlsx`：一行一台十五列。
//
// 报错原文只在失败那台填（成功那台不显示服务端提示）；分类取这一台的分类名（与屏幕、
// `report.txt` 同一套名字）。
func writeResultsXlsx(path string, l contract.Ledger, mode contract.Mode) error {
	if len(l.Items) == 0 {
		return nil
	}

	f := excelize.NewFile()
	defer func() { _ = f.Close() }()
	const sheet = "Results"
	if err := f.SetSheetName("Sheet1", sheet); err != nil {
		return err
	}
	st, err := resultsStyles(f)
	if err != nil {
		return err
	}

	headers := message.ResultsHeaders()
	maxWidths := make([]int, len(headers))
	for i, h := range headers {
		at, _ := excelize.CoordinatesToCellName(i+1, 1)
		if err := cell(f, sheet, at, h); err != nil {
			return err
		}
		if err := f.SetCellStyle(sheet, at, at, st.header); err != nil {
			return err
		}
		maxWidths[i] = message.DisplayWidth(h)
	}

	for idx, item := range l.Items {
		exit := ""
		if item.ExitCode != nil {
			exit = fmt.Sprintf("%d", *item.ExitCode)
		}
		errColumn := ""
		if !message.IsSuccess(item, mode) {
			errColumn = item.ErrorText
		}
		values := []any{
			// 序号从 0 起——老版那张表就是这样（`Seq` 是下标），A/B 逐格对得上才叫一致。
			idx, item.IP, portNumber(item.Port), item.User, item.Connected, exit,
			item.ConnectCost.Seconds(), item.Cost.Seconds(),
			item.Transfer.TotalBytes, item.Transfer.TotalFiles,
			item.Transfer.SuccessFiles, item.Transfer.FailFiles,
			item.Category, errColumn, item.Output,
		}
		for i, v := range values {
			at, _ := excelize.CoordinatesToCellName(i+1, idx+2)
			if err := cell(f, sheet, at, v); err != nil {
				return err
			}
			if err := f.SetCellStyle(sheet, at, at, st.body); err != nil {
				return err
			}
			if w := message.DisplayWidth(fmt.Sprintf("%v", v)); w > maxWidths[i] {
				maxWidths[i] = w
			}
		}
	}

	for i := range headers {
		width := float64(maxWidths[i] + 2)
		switch i {
		case resultsAutoColumns:
			width = resultsErrorWidth
		case resultsAutoColumns + 1:
			width = resultsOutputWidth
		}
		if err := f.SetColWidth(sheet, colName(i+1), colName(i+1), width); err != nil {
			return err
		}
	}

	last, _ := excelize.CoordinatesToCellName(len(headers), len(l.Items)+1)
	if err := f.AutoFilter(sheet, fmt.Sprintf("A1:%s", last), nil); err != nil {
		return err
	}
	return f.SaveAs(path)
}

// colName 列号（1 起）→ 列名（1→A、27→AA）。
func colName(idx int) string {
	name := ""
	for idx > 0 {
		idx--
		name = string(rune('A'+idx%26)) + name
		idx /= 26
	}
	return name
}
