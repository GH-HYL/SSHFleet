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

// fieldSpan 是一段占的字节区间，**引号不算在内**：遮罩只替换这一段，引号与逗号原样留着。
type fieldSpan struct{ start, end int }

// fieldSpans 按 CSV 规则把一行切成若干段，交回每段在原行里的区间。
//
// 为什么不用 `encoding/csv`：它只交回拆好的字符串，交不回「这段在原行第几个字节」，
// 而清单副本要做到「除了凭据那两段，其余字节跟原件一样」——BOM、引号、空白都得原样，
// 只能就地替换，不能拆开重拼。
//
// 认不出来（引号没闭合、引号后面还跟着别的东西）就交回 false，由调用方原样返回：
// 那是取值判定那头该拒绝的东西，不在这里猜。
func fieldSpans(line string) ([]fieldSpan, bool) {
	var spans []fieldSpan
	for i := 0; ; {
		if i < len(line) && line[i] == '"' {
			j, closed := i+1, false
			for j < len(line) {
				if line[j] == '"' {
					if j+1 < len(line) && line[j+1] == '"' { // "" 是段内的一个引号
						j += 2
						continue
					}
					closed = true
					break
				}
				j++
			}
			if !closed {
				return nil, false
			}
			spans = append(spans, fieldSpan{i + 1, j})
			i = j + 1
			if i >= len(line) {
				return spans, true
			}
			if line[i] != ',' {
				return nil, false
			}
		} else {
			j := i
			for j < len(line) && line[j] != ',' {
				j++
			}
			spans = append(spans, fieldSpan{i, j})
			if j >= len(line) {
				return spans, true
			}
			i = j
		}
		// 到这里 i 指着那个逗号：跳过它，行尾则补一段空的。
		i++
		if i >= len(line) {
			return append(spans, fieldSpan{i, i}), true
		}
	}
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
