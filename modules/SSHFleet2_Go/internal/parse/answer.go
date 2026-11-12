package parse

// 代填（`-a`）的取值：来源判定 → 读文本 → 读成行 → 一行一条代填 → 整表复查。
//
// 取值形态与 `-f` 完全同构：给一个 CSV 文件路径，或直接写一条内联代填；两者走同一处判定
// （`-f` 那一路在 nodelist.go 的 manifestText，`-a` 这一路在这里，判定口径相同），
// `;;` 与字面 `\n` 都只是普通字符（ADR-0023）。
//
// 行读法与清单共用 `csvRows`：`#` 注释行、空行、BOM、双引号、变长列的口径全部一致，
// 没有第二套规则。
//
// 列语义是代填自己的：第 1 列是代填内容，第 2 列起是触发词，列数自适应。

import (
	"encoding/csv"
	"errors"
	"os"
	"regexp"
	"strings"

	"sshfleet2/internal/contract"
	"sshfleet2/internal/criterion"
	"sshfleet2/internal/message"
)

// answerTable 把 `-a` 的值读成一张代填表。
//
// 空值交回空表：没给 `-a` 不是错。
func answerTable(raw string, regex, caseSensitive bool) ([]contract.Answer, *contract.CannotStart) {
	if raw == "" {
		return nil, nil
	}
	path, isFile, fail := answerSource(raw)
	if fail != nil {
		return nil, fail
	}

	text := raw
	if isFile {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, cliReason(contract.CannotStartReason{
				Kind:   contract.CannotStartAnswerFileUnreadable,
				Place:  path,
				Origin: criterion.SystemReason(err),
			})
		}
		if isBinaryContent(data) {
			return nil, cliReason(contract.CannotStartReason{Kind: contract.CannotStartAnswerFileBinary, Place: path})
		}
		text = string(data)
	}

	rows, lines, err := csvRows(text)
	if err != nil {
		var parseErr *csv.ParseError
		if errors.As(err, &parseErr) {
			where := ""
			if isFile {
				where = path
			}
			return nil, cliReason(contract.CannotStartReason{
				Kind:  contract.CannotStartAnswerSyntax,
				Place: where,
				Index: parseErr.Line,
			})
		}
		return nil, cliReason(contract.CannotStartReason{Kind: contract.CannotStartManifestRead, Origin: err.Error()})
	}

	answers := make([]contract.Answer, 0, len(rows))
	for i, row := range rows {
		entry, why := answerOf(row)
		if why != "" {
			where := ""
			if isFile {
				where = path
			}
			return nil, cliReason(contract.CannotStartReason{
				Kind:   contract.CannotStartAnswerRowInvalid,
				Place:  where,
				Index:  lines[i],
				Value:  strings.Join(row, ","),
				Origin: why,
			})
		}
		answers = append(answers, entry)
	}
	if len(answers) == 0 {
		where := ""
		if isFile {
			where = path
		}
		return nil, cliReason(contract.CannotStartReason{Kind: contract.CannotStartAnswerEmpty, Place: where})
	}
	if fail := validateAnswers(answers, regex, caseSensitive); fail != nil {
		return nil, fail
	}
	return answers, nil
}

// answerSource 判这次给的是文件还是内联那一条。
//
// 先按路径交给系统（原值原样给，不换分隔符、不去空白、不拼前缀）；不是文件再看值里有没有
// 列分隔符——一条代填至少要有「内容 + 触发词」两列。既有去路两句，逐字照老版。
func answerSource(raw string) (path string, isFile bool, fail *contract.CannotStart) {
	_, statErr := os.Stat(raw)
	if statErr == nil {
		// 目录也算「命中了路径」：后面 ReadFile 会报「是个目录」，那正是老版的口径
		// （判定只看系统认不认得这个值，不替它猜）。
		return raw, true, nil
	}
	if strings.ContainsAny(raw, "\r\n") {
		return "", false, cliReason(contract.CannotStartReason{Kind: contract.CannotStartAnswerInlineNewline})
	}
	if !strings.Contains(raw, ",") {
		return "", false, cliReason(contract.CannotStartReason{
			Kind:   contract.CannotStartAnswerGone,
			Place:  raw,
			Origin: criterion.SystemReason(statErr),
		})
	}
	return "", false, nil
}

// answerOf 解析一条代填：第 1 段是内容，其余各段是触发词。
//
// 第 1 段留空是合法写法：代表只发一个回车。交回的第二样是「为什么不合规」，空串表示过。
func answerOf(fields []string) (contract.Answer, string) {
	if len(fields) == 0 {
		return contract.Answer{}, message.AnswerRowNoTrigger()
	}
	entry := contract.Answer{Value: fields[0], Triggers: fields[1:]}
	if len(entry.Triggers) == 0 {
		return contract.Answer{}, message.AnswerRowNoTrigger()
	}
	for i, trigger := range entry.Triggers {
		if strings.TrimSpace(trigger) == "" {
			return contract.Answer{}, message.AnswerRowEmptyTrigger(i + 2)
		}
	}
	return entry, ""
}

// validateAnswers 整表复查：触发词是唯一的门控——没有它等于无条件盲填。
//
// 正则模式下顺带预编译触发词：语法不对在本地就拦下，不留到运行期静默不匹配
// （那时用户只会看到「触发词未命中」，而真原因是触发词自己写错了）。
func validateAnswers(answers []contract.Answer, regex, caseSensitive bool) *contract.CannotStart {
	for i, entry := range answers {
		if len(entry.Triggers) == 0 {
			return cliReason(contract.CannotStartReason{
				Kind:  contract.CannotStartAnswerMissingTrigger,
				Index: i + 1,
				Value: entry.Value,
			})
		}
		if !regex {
			continue
		}
		for _, kw := range entry.Triggers {
			pattern := kw
			if !caseSensitive {
				pattern = "(?i)" + kw
			}
			if _, err := regexp.Compile(pattern); err != nil {
				return cliReason(contract.CannotStartReason{
					Kind:   contract.CannotStartAnswerBadRegex,
					Index:  i + 1,
					Value:  kw,
					Origin: err.Error(),
				})
			}
		}
	}
	return nil
}

// isBinaryContent 内容里有没有 NUL 字节：有就按二进制处理，不硬当文本读。
func isBinaryContent(data []byte) bool {
	for _, b := range data {
		if b == 0 {
			return true
		}
	}
	return false
}
