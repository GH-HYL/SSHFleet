package nodelist

import (
	"encoding/csv"
	"errors"
	"fmt"
	"os"
	"strings"

	"sshfleet/internal/common"
)

// readCSVRows 读取并清洗清单行：跳空行 / `#` 注释行、判空、移除表头。
//
// 解析走 common.ReadCSVRows——与 `-a` 代填的取值共用同一份实现，所以 CSV 口径
// （双引号、BOM、注释行、空行、变长列）两处天然一致，不存在第二套规则。
// 清单自己的两件事留在本函数里：内联值的字面 `\n` 补成换行、第一行是不是表头。
//
// 提示经 in 落注入的输出流（不再直写 os.Stdout，便于测试捕获）。
func readCSVRows(csvPath string, isInline bool, in *common.Interactor) ([][]string, error) {
	text, err := csvText(csvPath, isInline)
	if err != nil {
		return nil, err
	}
	rows, err := common.ReadCSVRows(text)
	if err != nil {
		return nil, csvReadError(csvPath, isInline, err)
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("CSV 中未解析出任何有效节点，请检查节点文件或内联文本内容")
	}

	// 清单侧只取字段：报错用的是逐节点的序号，行号口径见 resolveNodes
	infos := make([][]string, 0, len(rows))
	for _, row := range rows {
		infos = append(infos, row.Fields)
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

// csvText 按来源取到要解析的文本。
//
// 内联值先把字面 `\n`（反斜杠 + n）补成真实换行——命令行里传不进换行，多行
// 清单只能这么写；文件内容是原样读进来的。补完之后两者走同一条解析路径。
func csvText(csvPath string, isInline bool) (string, error) {
	if isInline {
		return common.ExpandEscapedNewlines(csvPath), nil
	}
	data, err := os.ReadFile(csvPath)
	if err != nil {
		return "", fmt.Errorf("读取内容时发生错误: %v", err)
	}
	return string(data), nil
}

// csvReadError CSV 语法错的文案。Go 的原文是行话，用户只需要知道哪一行、
// 引号要成对（与代填侧同口径，见 cli/answer.go）。
func csvReadError(csvPath string, isInline bool, err error) error {
	var parseErr *csv.ParseError
	if !errors.As(err, &parseErr) {
		return fmt.Errorf("读取内容时发生错误: %v", err)
	}
	where := "内联节点信息的"
	if !isInline {
		where = fmt.Sprintf("%s 的", csvPath)
	}
	return fmt.Errorf(
		"读取节点清单失败：%s第 %d 行格式读不了——CSV 的双引号要成对出现\n"+
			"提示：值里要用引号时，把里面的引号写成两个", where, parseErr.Line)
}
