package nodelist

import (
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"strings"

	"sshfleet/internal/common"
)

// readCSVRows 读取并清洗 CSV 行：跳空行 / `#` 注释行、判空、移除表头。
// FieldsPerRecord = -1：旧版容忍变长行（缺列自动补空），Go 默认要求每行列数一致，
// 不设会直接报错（HANDOVER 已知坑）。
// 提示经 in 落注入的输出流（不再直写 os.Stdout，便于测试捕获）。
func readCSVRows(csvPath string, isInline bool, in *common.Interactor) ([][]string, error) {
	var reader *csv.Reader
	if isInline {
		reader = csv.NewReader(strings.NewReader(csvPath))
	} else {
		f, err := os.Open(csvPath)
		if err != nil {
			return nil, fmt.Errorf("读取内容时发生错误: %v", err)
		}
		defer f.Close()
		reader = csv.NewReader(f)
	}
	reader.FieldsPerRecord = -1 // 容忍变长行（对位旧 Python csv.reader）

	var infos [][]string
	for {
		row, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("读取内容时发生错误: %v", err)
		}
		// 跳过空行和注释行（对位旧逻辑：row 为空，或首列以 # 开头且非空）
		if len(row) == 0 {
			continue
		}
		if strings.HasPrefix(row[0], "#") && strings.TrimSpace(row[0]) != "" {
			continue
		}
		infos = append(infos, row)
	}

	if len(infos) == 0 {
		return nil, fmt.Errorf("CSV 中未解析出任何有效节点，请检查节点文件或内联文本内容")
	}

	// 表头识别（spec 实现层差异，落实 D13）：首行首列严格 IPv4 解析失败
	// 且该行含逗号 → 判为表头移除；否则报错。
	first := infos[0]
	if !isStrictIPv4(first[0]) {
		joined := strings.Join(first, ",")
		if strings.Contains(joined, ",") {
			infos = infos[1:]
			in.Notice(fmt.Sprintf("%s[INFO]%s%s [function:read_nodes_infos]%s 第一行不是IP格式，已移除表头行\n", colorCyan, colorReset, colorYellow, colorReset))
			if len(infos) == 0 {
				return nil, fmt.Errorf("CSV 中未解析出任何有效节点，请检查节点文件或内联文本内容")
			}
		} else {
			return nil, fmt.Errorf("CSV 中未解析出任何有效节点，请检查节点文件或内联文本内容")
		}
	}

	return infos, nil
}
