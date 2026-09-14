// xlsx 输出（对位旧 xlsx.py，openpyxl → github.com/xuri/excelize/v2）。
package output

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/xuri/excelize/v2"

	"sshfleet/internal/batch"
	"sshfleet/internal/config"
	"sshfleet/internal/result"
	"sshfleet/internal/ssh"
)

// WriteOutputXlsx 生成 <归档目录>/<paths.output_xlsx>：3 列（IP地址、事件类型、内容详情），
// 一条结果展开为「连接 / 执行(上传|下载) / 分类」若干行（对位旧 format_output_to_xlsx）。
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

	headerStyle, err := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true, Color: "FFFFFF", Size: 12},
		Fill:      excelize.Fill{Type: "pattern", Color: []string{"366092"}, Pattern: 1},
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center"},
	})
	if err != nil {
		return err
	}

	headers := []string{"IP地址", "事件类型", "内容详情"}
	for i, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		if err := f.SetCellValue(sheet, cell, h); err != nil {
			return err
		}
		if err := f.SetCellStyle(sheet, cell, cell, headerStyle); err != nil {
			return err
		}
	}

	row := 1
	put := func(ip, event, detail string) error {
		row++
		for i, v := range []string{ip, event, detail} {
			cell, _ := excelize.CoordinatesToCellName(i+1, row)
			if err := f.SetCellValue(sheet, cell, v); err != nil {
				return err
			}
		}
		return nil
	}

	for _, r := range results.Items {
		if err := put(r.IP, "连接", FormatConnStatus(r.ConnectSuccess, r.ConnectCostTime)); err != nil {
			return err
		}
		if r.ConnectSuccess {
			status := "失败"
			if r.ExitCode != nil && *r.ExitCode == 0 {
				status = "成功"
			}
			detail := fmt.Sprintf("%s - %.3fs", status, r.ExecCostTime)
			if out := strings.TrimSpace(r.Output); out != "" {
				detail += "\n" + out
			}
			if err := put(r.IP, ActionName(mode), detail); err != nil {
				return err
			}
		} else if r.Error != nil {
			if err := put(r.IP, "错误", *r.Error); err != nil {
				return err
			}
		}
		if err := put(r.IP, "分类", categoryOf(r)); err != nil {
			return err
		}
	}

	if err := f.SetColWidth(sheet, "A", "A", 18); err != nil {
		return err
	}
	if err := f.SetColWidth(sheet, "B", "B", 14); err != nil {
		return err
	}
	if err := f.SetColWidth(sheet, "C", "C", 80); err != nil {
		return err
	}
	if err := f.AutoFilter(sheet, fmt.Sprintf("A1:C%d", row), nil); err != nil {
		return err
	}
	return f.SaveAs(filepath.Join(archiveDir, cfg.Paths.OutputXlsx))
}

// WriteResultsXlsx 生成 <归档目录>/<paths.results_xlsx>：结果逐条固化为一行（表头取字段名）。
func WriteResultsXlsx(archiveDir string, results *batch.Results, cfg *config.Config, mode string, kw *result.Keywords, categoryOf func(ssh.Result) string) error {
	if results.Len() == 0 {
		return nil
	}

	f := excelize.NewFile()
	defer func() { _ = f.Close() }()
	const sheet = "Sheet1"

	headers := []string{"seq", "ip", "port", "user", "connect_success", "exit_code", "connect_cost_time",
		"exec_cost_time", "total_bytes", "total_files", "success_files", "failed_files", "分类", "error", "output"}
	headerStyle, err := f.NewStyle(&excelize.Style{
		Font: &excelize.Font{Bold: true, Color: "FFFFFF"},
		Fill: excelize.Fill{Type: "pattern", Color: []string{"366092"}, Pattern: 1},
	})
	if err != nil {
		return err
	}
	for i, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		if err := f.SetCellValue(sheet, cell, h); err != nil {
			return err
		}
		if err := f.SetCellStyle(sheet, cell, cell, headerStyle); err != nil {
			return err
		}
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
			r.SuccessFiles, r.FailedFiles, categoryOf(r), errText, r.Output,
		}
		for i, v := range values {
			cell, _ := excelize.CoordinatesToCellName(i+1, idx+2)
			if err := f.SetCellValue(sheet, cell, v); err != nil {
				return err
			}
		}
	}
	return f.SaveAs(filepath.Join(archiveDir, cfg.Paths.ResultsXlsx))
}
