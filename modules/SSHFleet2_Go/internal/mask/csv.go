package mask

// CSV 的读与写各留一个入口：内联记录与清单副本都要按 CSV 规则走（值可能整段带引号、
// 引号里可能包着逗号，按裸逗号切会把段号切错、把密码漏在外面）。两处只差「认不认 `#`
// 注释」，所以读法共用一份、用一个参数分开；写法完全一样，只留一个。

import (
	"encoding/csv"
	"strings"
)

// newCSVReader 读一段 CSV 文本。comment 为真时把 `#` 开头的行当注释跳过（内联记录用），
// 为假时整行就是数据（清单逐行处理用）。
func newCSVReader(text string, comment bool) *csv.Reader {
	r := csv.NewReader(strings.NewReader(strings.TrimPrefix(text, "\ufeff")))
	r.FieldsPerRecord = -1
	r.LazyQuotes = true
	if comment {
		r.Comment = '#'
	}
	return r
}

// joinCSV 把一条记录按 CSV 规则拼回一行（不带结尾换行）；拼不出来时交回错误，
// 由调用方决定「原样返回」还是「报失败」。
func joinCSV(fields []string) (string, error) {
	var b strings.Builder
	w := csv.NewWriter(&b)
	if err := w.Write(fields); err != nil {
		return "", err
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return "", err
	}
	return strings.TrimSuffix(b.String(), "\n"), nil
}
