package parse

import (
	"encoding/csv"
	"errors"
	"fmt"
	"os"
	"strings"

	"sshfleet2/internal/contract"
	"sshfleet2/internal/criterion"
)

// 读清单：把 `-f` 那个值变成一张行表。
//
// 两种形态共用一个入口——一份 CSV 文件的路径，或一条内联记录（词表「CSV 取值」）。
// 判定只看原值能不能被系统找到，不认后缀名；判定与文件内容自检的先后照老版：
// 先判它是文件还是记录，再查文件读得了、非空、非二进制。

// manifest 是读出来的清单：一行一台的原始字段，外加要打给使用者的那几句提示。
type manifest struct {
	rows          [][]string
	inline        bool
	headerDropped bool // 首行被当表头移除过（「第一行不是 IP，已当表头移除」那句提示）
}

// readManifest 读 `-f` 的值。
func readManifest(raw string) (manifest, *contract.CannotStart) {
	text, inline, fail := manifestText(raw)
	if fail != nil {
		return manifest{}, fail
	}

	rows, _, err := criterion.ReadCSVRows(text)
	if err != nil {
		return manifest{}, cannotStartReason("nodelist", manifestSyntaxError(raw, inline, err))
	}
	if len(rows) == 0 {
		return manifest{}, cannotStartReason("nodelist", contract.CannotStartReason{Kind: contract.CannotStartNoNodeParsed})
	}

	m := manifest{rows: rows, inline: inline}

	// 表头识别：首行首列不是 IPv4、且这一行里有逗号，就当表头移除（老版 D13 的落法）。
	// 报了那一行提示，使用者才知道「为什么少了一台」。
	if len(rows) > 0 && !criterion.IsIPv4(rows[0][0]) {
		if len(rows[0]) > 1 {
			m.rows = rows[1:]
			m.headerDropped = true
			if len(m.rows) == 0 {
				return manifest{}, cannotStartReason("nodelist", contract.CannotStartReason{Kind: contract.CannotStartNoNodeParsed})
			}
		} else {
			return manifest{}, cannotStartReason("nodelist", contract.CannotStartReason{Kind: contract.CannotStartNoNodeParsed})
		}
	}
	return m, nil
}

// manifestText 判这次给的是文件还是内联记录，取到要解析的文本。
//
// 报错的三段话（问题 / 原因 / 提示）逐字照老版：原因取系统那一句——「找不到」与
// 「没权限」系统比我们分得清（ADR-0025 决定 6）。
func manifestText(raw string) (string, bool, *contract.CannotStart) {
	// 空串不在这里报：没给 `-f` 是第三层的必填，由闸门说自己的话（ADR-0052 决定二）。
	// 本站只在拿到一个值时判它是文件还是内联记录——空值压根走不到这里。
	info, statErr := os.Stat(raw)
	if statErr == nil && !info.IsDir() {
		data, err := os.ReadFile(raw)
		if err != nil {
			return "", false, cliReason(contract.CannotStartReason{
				Kind:   contract.CannotStartCSVUnreadable,
				Place:  raw,
				Origin: criterion.SystemReason(err),
			})
		}
		if fail := textMaterial(data, contract.CannotStartCSVEmpty, contract.CannotStartCSVBinary, raw); fail != nil {
			return "", false, fail
		}
		return string(data), false, nil
	}

	// 不是文件：要么是一条内联记录，要么两样都不是。
	if strings.ContainsAny(raw, "\r\n") {
		return "", true, cliReason(contract.CannotStartReason{
			Kind: contract.CannotStartManifestInlineNewline,
		})
	}
	if !criterion.LooksLikeInlineNodeRecord(raw) {
		reason := contract.CannotStartReason{
			Kind:  contract.CannotStartManifestPathMissing,
			Place: raw,
		}
		if statErr != nil {
			reason.Kind = contract.CannotStartManifestStatFailed
			reason.Origin = criterion.SystemReason(statErr)
		}
		return "", true, cliReason(reason)
	}
	return raw, true, nil
}

// readFailure 是「读清单这一步栽了」的那一句：系统原话照贴，不自己另写一句
// （ADR-0025 决定六）。
//
// 一处成形：空串当路径交给系统那一句、CSV 读不下去又不是语法错的那一句，本来就是同一句话，
// 分开各拼一次，改口径就会改漏一处。
func readFailure(err error) string {
	return fmt.Sprintf("读取内容时发生错误: %v", err)
}

// manifestSyntaxError 是 CSV 语法错的那句话：说清哪一行、引号要成对。
// Go 的原文是行话，使用者看了不知道怎么办。
//
// 说得出行号时交「内联／文件 ＋ 行号」，说不出来就退回「读清单栽了」那一类（系统原话照贴）。
func manifestSyntaxError(raw string, inline bool, err error) contract.CannotStartReason {
	var parseErr *csv.ParseError
	if !errors.As(err, &parseErr) {
		return contract.CannotStartReason{
			Kind:   contract.CannotStartManifestRead,
			Origin: err.Error(),
		}
	}
	if inline {
		return contract.CannotStartReason{
			Kind:  contract.CannotStartManifestSyntaxInline,
			Index: parseErr.Line,
		}
	}
	return contract.CannotStartReason{
		Kind:  contract.CannotStartManifestSyntaxFile,
		Place: raw,
		Index: parseErr.Line,
	}
}
