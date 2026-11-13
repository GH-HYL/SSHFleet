package criterion

import (
	"encoding/csv"
	"errors"
	"io"
	"strings"
)

// CSV 取值：一段文本怎么读成逐条记录。
//
// 口径（清单与代填共用同一份，不因来源是文件还是内联而变，ADR-0023 决定 5）：
//   - 剥开头的 UTF-8 BOM（记事本存 CSV 默认带它，留着会让首行的 `#` 注释失效）
//   - `#` 在行首的整行跳过（内联记录也按这条读）
//   - 空行跳过
//   - 双引号成对时，引号里可以包住逗号与换行
//   - 每行列数不限，缺的列由调用方按自己的口径补
//
// **引号一律严格**：不成对的引号读不出，没有「将就着读」的宽松档。老版只有一个
// `ReadCSVRows`，取值判定与脱敏都调它；新版曾给脱敏那份单开 `LazyQuotes`，同一组畸形值
// 两处结论相反（本议题 `audit.md` §三·1），这里收回来。
func ReadCSVRows(text string) ([][]string, []int, error) {
	src := strings.TrimPrefix(text, "\ufeff")
	r := csv.NewReader(strings.NewReader(src))
	r.FieldsPerRecord = -1
	r.Comment = '#'

	var (
		rows  [][]string
		lines []int
	)
	for {
		rec, err := r.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, nil, err
		}
		// 行号取自读入器自己的记录位置，不靠事后回溯（重复行会回溯到第一个，报错就指到别处）。
		line, _ := r.FieldPos(0)
		rows = append(rows, rec)
		lines = append(lines, line)
	}
	return rows, lines, nil
}

// LooksLikeInlineNodeRecord 一个值能不能当「一条内联节点记录」读：恰好一条、首列是 IPv4。
//
// 取值判定（`-f` 收不收）与脱敏（要不要遮）都认这一条，不会出现「放行认得出、脱敏认不出」
// （ADR-0023 决定 5）。用 CSV 读法而不是自己按逗号切：手切看不出引号里包着的逗号，也认不出
// 引号没闭合这种「读不成一条」的值。
func LooksLikeInlineNodeRecord(raw string) bool {
	_, ok := SingleNodeRecord(raw)
	return ok
}

// SingleNodeRecord 同一条判据，只是把读出来的字段一并交回——遮罩那一侧要按段号找凭据，
// 让它自己再读一遍就成了两处口径。读法与形状判定都在这里，调用方只拿结果。
func SingleNodeRecord(raw string) ([]string, bool) {
	rows, _, err := ReadCSVRows(raw)
	if err != nil || len(rows) != 1 || len(rows[0]) == 0 {
		return nil, false
	}
	if !IsIPv4(rows[0][0]) {
		return nil, false
	}
	return rows[0], true
}
