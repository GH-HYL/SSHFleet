package parse

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"sshfleet2/internal/contract"
	"sshfleet2/internal/message"
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

	rows, _, err := csvRows(text)
	if err != nil {
		return manifest{}, cannotStart("nodelist", "", manifestSyntaxError(raw, inline, err))
	}
	if len(rows) == 0 {
		return manifest{}, cannotStartReason("nodelist", contract.CannotStartReason{Kind: contract.CannotStartNoNodeParsed})
	}

	m := manifest{rows: rows, inline: inline}

	// 表头识别：首行首列不是 IPv4、且这一行里有逗号，就当表头移除（老版 D13 的落法）。
	// 报了那一行提示，使用者才知道「为什么少了一台」。
	if len(rows) > 0 && !isIPv4(strings.TrimSpace(rows[0][0])) {
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
	if raw == "" {
		// 老版这里也是把空串当路径交给系统，报的就是系统那一句
		// （`读取内容时发生错误: open : no such file or directory`）。照办，不另写一句话。
		_, err := os.ReadFile(raw)
		return "", false, cannotStartReason("nodelist", contract.CannotStartReason{Kind: contract.CannotStartManifestRead, Origin: err.Error()})
	}

	info, statErr := os.Stat(raw)
	if statErr == nil && !info.IsDir() {
		data, err := os.ReadFile(raw)
		if err != nil {
			return "", false, materialFail(message.Problem(
				fmt.Sprintf("-f 参数指定的 CSV 文件读不到：%s", raw),
				message.SystemReason(err),
				"请检查文件是否存在、当前用户有没有读权限"))
		}
		if len(data) == 0 {
			return "", false, materialFail(message.ProblemHint(
				fmt.Sprintf("-f 参数指定的 CSV 文件为空：%s", raw),
				"一行写一台机器，只写 IP 就能跑（端口、用户名、密码走配置）"))
		}
		return string(data), false, nil
	}

	// 不是文件：要么是一条内联记录，要么两样都不是。
	if strings.ContainsAny(raw, "\r\n") {
		return "", true, materialFail(message.ProblemHint(
			"-f 的内联值里不能有换行：内联只写一条",
			"把节点写进 CSV 文件，再用 -f 指向它"))
	}
	if !looksLikeInlineNode(raw) {
		reason := "找不到这个文件"
		if statErr != nil {
			reason = message.SystemReason(statErr)
		}
		return "", true, materialFail(message.Problem(
			fmt.Sprintf("-f 参数指定的文件读不到：%s", raw),
			reason,
			message.ManifestGoneHint))
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

// csvRows 把文本读成行：剥 BOM、`#` 注释行跳过、空行跳过、列数不限、引号成对时可含逗号与换行。
//
// 这套口径与 `-a` 代填共用：清单与代填不再各写一套规则。
//
// 第二个返回值是每一行在**文本里的行号**（1 起，空行与注释行照样占号）：报错要说清
// 「第几行」，而跳过空行之后下标与行号就对不上了。行号取自读入器自己的记录位置，
// 不靠事后回溯（重复行会回溯到第一个，报错就指到别处）。
func csvRows(text string) ([][]string, []int, error) {
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
		line, _ := r.FieldPos(0)
		rows = append(rows, rec)
		lines = append(lines, line)
	}
	return rows, lines, nil
}

// manifestSyntaxError 是 CSV 语法错的那句话：说清哪一行、引号要成对。
// Go 的原文是行话，使用者看了不知道怎么办。
func manifestSyntaxError(raw string, inline bool, err error) string {
	var parseErr *csv.ParseError
	if !errors.As(err, &parseErr) {
		return readFailure(err)
	}
	where := "内联节点信息的"
	if !inline {
		where = raw + " 的"
	}
	return message.ProblemHint(
		fmt.Sprintf("读取节点清单失败：%s第 %d 行格式读不了——CSV 的双引号要成对出现", where, parseErr.Line),
		"值里要用引号时，把里面的引号写成两个")
}

// looksLikeInlineNode 这个值像不像一条内联节点记录：按 CSV 读法取首字段，是 IPv4 就算。
//
// 用 CSV 读法而不是自己按逗号切：手切看不出引号里包着的逗号，也认不出引号没闭合。
func looksLikeInlineNode(raw string) bool {
	rows, _, err := csvRows(raw)
	if err != nil || len(rows) != 1 || len(rows[0]) == 0 {
		return false
	}
	return isIPv4(strings.TrimSpace(rows[0][0]))
}

// isIPv4 严格认 IPv4 字面量：四段、每段 0-255、不认前导零以外的花样。
//
// 与老版同一条判据（旧版正则不校验每段范围，已修正）。
func isIPv4(s string) bool {
	if s == "" {
		return false
	}
	parts := strings.Split(s, ".")
	if len(parts) != 4 {
		return false
	}
	for _, p := range parts {
		if p == "" || len(p) > 3 {
			return false
		}
		n := 0
		for _, r := range p {
			if r < '0' || r > '9' {
				return false
			}
			n = n*10 + int(r-'0')
		}
		if n > 255 {
			return false
		}
	}
	return true
}
