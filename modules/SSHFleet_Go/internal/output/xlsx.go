// xlsx 输出（对位旧 xlsx.py，openpyxl → github.com/xuri/excelize/v2）。
package output

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/xuri/excelize/v2"

	"sshfleet/internal/batch"
	"sshfleet/internal/config"
	"sshfleet/internal/result"
	"sshfleet/internal/ssh"
)

// ---- 内容清理（对位旧 text_utils.clean_for_excel）----

var (
	// ANSI 转义：CSI（颜色/光标）、OSC（标题等）、以及其余 ESC 序列。
	ansiEscapeRe = regexp.MustCompile(
		`\x1b\[[0-9;]*[a-zA-Z]` +
			`|\x1b\][^\x07]*(?:\x07|\x1b\\)` +
			`|\x1b[@-_][0-?]*[-/]*[@-~]`)
	// 非打印/零宽字符（保留 \t \n \r）：C0/C1 控制符、零宽与格式字符。
	// 注意 C1 段必须写 \u007f-\u009f（码点）——Go 字符串里 \x7f-\x9f 是**原始字节**，
	// 0x9f 是非法 UTF-8，正则编译会直接失败。
	excelInvisibleRe = regexp.MustCompile(
		"[\x00-\x08\x0b\x0c\x0e-\x1f\u007f-\u009f" +
			"\u200b-\u200f\u2028-\u202f\u2060-\u206f\ufeff]")
)

// cleanForExcel 清理写入 Excel 的文本（对位旧 clean_for_excel）：
// 非法 UTF-8 替换 → 去 ANSI 转义 → 去非打印/零宽字符 → 统一换行 → 行首 `=` 前补空格
// （防被判成公式）→ 去掉首尾空行。
//
// 为什么要清：远端命令输出是任意字节流（颜色转义、进度条控制符、二进制片段），
// 终端与 output.txt 能原样吃下，但 xlsx 是 XML——控制字符与非法 UTF-8 会直接把文件写坏。
// 采集侧刻意不做任何处理（spec：原生交给终端/txt），故清理只在这里做，且是全量唯一入口。
func cleanForExcel(s string) string {
	s = strings.ToValidUTF8(s, "\uFFFD")
	s = ansiEscapeRe.ReplaceAllString(s, "")
	s = excelInvisibleRe.ReplaceAllString(s, "")
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

// setCell 写单元格的唯一入口：字符串一律先过 cleanForExcel，避免任何来源的原文
// （远端输出、错误原文、兜底分类）绕过清理写坏 xlsx；数字/布尔原样写入。
func setCell(f *excelize.File, sheet, cell string, v any) error {
	if s, ok := v.(string); ok {
		v = cleanForExcel(s)
	}
	return f.SetCellValue(sheet, cell, v)
}

// ---- 样式 ----

type xlsxStyles struct {
	header, detail, separator, cell, headerBordered int
}

// newOutputStyles 旧 output.xlsx 的样式：表头深蓝底白字加粗居中；输出明细顶端对齐自动换行；
// 结果之间一条浅蓝分隔行。
func newOutputStyles(f *excelize.File) (*xlsxStyles, error) {
	st := &xlsxStyles{}
	var err error
	if st.header, err = f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true, Color: "FFFFFF", Size: 12},
		Fill:      excelize.Fill{Type: "pattern", Color: []string{"366092"}, Pattern: 1},
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center"},
	}); err != nil {
		return nil, err
	}
	if st.detail, err = f.NewStyle(&excelize.Style{
		Alignment: &excelize.Alignment{Vertical: "top", WrapText: true},
	}); err != nil {
		return nil, err
	}
	if st.separator, err = f.NewStyle(&excelize.Style{
		Fill: excelize.Fill{Type: "pattern", Color: []string{"D9E1F2"}, Pattern: 1},
	}); err != nil {
		return nil, err
	}
	return st, nil
}

// newResultsStyles 旧 results.xlsx 的样式：表头蓝底白字加粗居中 + 细边框，数据单元格细边框。
func newResultsStyles(f *excelize.File) (*xlsxStyles, error) {
	thin := []excelize.Border{
		{Type: "left", Color: "000000", Style: 1},
		{Type: "right", Color: "000000", Style: 1},
		{Type: "top", Color: "000000", Style: 1},
		{Type: "bottom", Color: "000000", Style: 1},
	}
	st := &xlsxStyles{}
	var err error
	if st.headerBordered, err = f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true, Color: "FFFFFF"},
		Fill:      excelize.Fill{Type: "pattern", Color: []string{"4472C4"}, Pattern: 1},
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center"},
		Border:    thin,
	}); err != nil {
		return nil, err
	}
	if st.cell, err = f.NewStyle(&excelize.Style{Border: thin}); err != nil {
		return nil, err
	}
	return st, nil
}

// WriteOutputXlsx 生成 <归档目录>/<paths.output_xlsx>：3 列（IP地址、事件类型、内容详情）。
// 逐条结果展开为若干行（对位旧 format_output_to_xlsx）：
//
//	IP | 连接: 成功 - X.XXXs |
//	IP | 执行(上传|下载): 成功 - X.XXXs |
//	IP | 标准输出和错误输出 | 输出第 N 行（每条输出独占一行、顶端对齐自动换行）
//	IP | 分类: 分类名 |
//	（浅蓝分隔行）
func WriteOutputXlsx(archiveDir string, results *batch.Results, cfg *config.Config, mode string, kw *result.Keywords, categoryOf func(ssh.Result) string) error {
	if results.Len() == 0 {
		return nil
	}

	f := excelize.NewFile()
	defer func() { _ = f.Close() }()
	const sheet = "执行日志"
	if err := f.SetSheetName("Sheet1", sheet); err != nil {
		return err
	}
	st, err := newOutputStyles(f)
	if err != nil {
		return err
	}

	headers := []string{"IP地址", "事件类型", "内容详情"}
	for i, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		if err := setCell(f, sheet, cell, h); err != nil {
			return err
		}
		if err := f.SetCellStyle(sheet, cell, cell, st.header); err != nil {
			return err
		}
	}

	row := 1
	put := func(ip, event, detail string) error {
		row++
		for i, v := range []string{ip, event, detail} {
			cell, _ := excelize.CoordinatesToCellName(i+1, row)
			if err := setCell(f, sheet, cell, v); err != nil {
				return err
			}
		}
		return nil
	}

	for _, r := range results.Items {
		if err := put(r.IP, FormatConnStatus(r.ConnectSuccess, r.ConnectCostTime), ""); err != nil {
			return err
		}

		status := "失败"
		if r.ExitCode != nil && *r.ExitCode == 0 {
			status = "成功"
		}
		if err := put(r.IP, fmt.Sprintf("%s: %s - %.3fs", ActionName(mode), status, r.ExecCostTime), ""); err != nil {
			return err
		}

		// 连接失败：补一行错误详情（旧版没这行，错误原文只能从 results.xlsx 的 error 列看到；
		// 用户 2026-09-15 要求保留，便于只开 output.xlsx 时也能定位原因）
		if !r.ConnectSuccess && r.Error != nil && *r.Error != "" {
			if err := put(r.IP, "错误", *r.Error); err != nil {
				return err
			}
		}

		for _, line := range strings.Split(cleanForExcel(r.Output), "\n") {
			if strings.TrimSpace(line) == "" {
				continue
			}
			row++
			ipCell, _ := excelize.CoordinatesToCellName(1, row)
			eventCell, _ := excelize.CoordinatesToCellName(2, row)
			detailCell, _ := excelize.CoordinatesToCellName(3, row)
			if err := setCell(f, sheet, ipCell, r.IP); err != nil {
				return err
			}
			if err := setCell(f, sheet, eventCell, "标准输出和错误输出"); err != nil {
				return err
			}
			if err := setCell(f, sheet, detailCell, strings.TrimSpace(line)); err != nil {
				return err
			}
			if err := f.SetCellStyle(sheet, detailCell, detailCell, st.detail); err != nil {
				return err
			}
		}

		if err := put(r.IP, "分类: "+categoryOf(r), ""); err != nil {
			return err
		}

		// 分隔行：整行浅蓝填充，结果之间一眼可分
		row++
		if err := f.SetCellStyle(sheet,
			fmt.Sprintf("A%d", row), fmt.Sprintf("C%d", row), st.separator); err != nil {
			return err
		}
	}

	for i, width := range []float64{15, 20, 60} {
		if err := f.SetColWidth(sheet, colName(i+1), colName(i+1), width); err != nil {
			return err
		}
	}
	if err := f.AutoFilter(sheet, fmt.Sprintf("A1:C%d", row), nil); err != nil {
		return err
	}
	// 冻结首行
	if err := f.SetPanes(sheet, &excelize.Panes{
		Freeze: true, YSplit: 1, TopLeftCell: "A2", ActivePane: "bottomLeft",
	}); err != nil {
		return err
	}
	return f.SaveAs(filepath.Join(archiveDir, cfg.Paths.OutputXlsx))
}

// resultsFixedWidth N、O 两列（error / output）的固定列宽：
// 这两列是长短不一的自由文本，不参与自适应，否则会撑出几百字符宽的列。
const resultsFixedWidth = 50

// WriteResultsXlsx 生成 <归档目录>/<paths.results_xlsx>：结果逐条固化为一行（表头取字段名）。
// 对位旧 format_dict_list_to_xlsx：表头带细边框、数据单元格带细边框、自动筛选；
// A–M 列（不含自由文本的 N/O）按内容做一次「最合适的列宽」（用户 2026-09-15 要求）。
func WriteResultsXlsx(archiveDir string, results *batch.Results, cfg *config.Config, mode string, kw *result.Keywords, categoryOf func(ssh.Result) string) error {
	if results.Len() == 0 {
		return nil
	}

	f := excelize.NewFile()
	defer func() { _ = f.Close() }()
	const sheet = "Results"
	if err := f.SetSheetName("Sheet1", sheet); err != nil {
		return err
	}
	st, err := newResultsStyles(f)
	if err != nil {
		return err
	}

	headers := []string{"seq", "ip", "port", "user", "connect_success", "exit_code", "connect_cost_time",
		"exec_cost_time", "total_bytes", "total_files", "success_files", "failed_files", "分类", "error", "output"}
	maxWidths := make([]int, len(headers))
	for i, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		if err := setCell(f, sheet, cell, h); err != nil {
			return err
		}
		if err := f.SetCellStyle(sheet, cell, cell, st.headerBordered); err != nil {
			return err
		}
		maxWidths[i] = DisplayWidth(h)
	}

	for idx, r := range results.Items {
		exit := ""
		if r.ExitCode != nil {
			exit = fmt.Sprintf("%d", *r.ExitCode)
		}
		errText := ""
		if r.Error != nil {
			errText = *r.Error
		}
		values := []any{
			r.Seq, r.IP, r.Port, r.User, r.ConnectSuccess, exit,
			r.ConnectCostTime, r.ExecCostTime, r.TotalBytes, r.TotalFiles,
			r.SuccessFiles, r.FailedFiles, categoryOf(r),
			cleanForExcel(errText), cleanForExcel(r.Output),
		}
		for i, v := range values {
			cell, _ := excelize.CoordinatesToCellName(i+1, idx+2)
			if err := setCell(f, sheet, cell, v); err != nil {
				return err
			}
			if err := f.SetCellStyle(sheet, cell, cell, st.cell); err != nil {
				return err
			}
			if w := DisplayWidth(fmt.Sprintf("%v", v)); w > maxWidths[i] {
				maxWidths[i] = w
			}
		}
	}

	// 列宽：A–M 自适应（内容 + 2 列留白），N/O 固定宽度
	for i := range headers {
		width := float64(resultsFixedWidth)
		if i < resultsFixedWidthColumns {
			width = float64(maxWidths[i] + 2)
		}
		if err := f.SetColWidth(sheet, colName(i+1), colName(i+1), width); err != nil {
			return err
		}
	}

	last, _ := excelize.CoordinatesToCellName(len(headers), results.Len()+1)
	if err := f.AutoFilter(sheet, fmt.Sprintf("A1:%s", last), nil); err != nil {
		return err
	}
	return f.SaveAs(filepath.Join(archiveDir, cfg.Paths.ResultsXlsx))
}

// resultsFixedWidthColumns 参与自适应列宽的列数：A–M 共 13 列，N/O 固定宽度。
const resultsFixedWidthColumns = 13

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
