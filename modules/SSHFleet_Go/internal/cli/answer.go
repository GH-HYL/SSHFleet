package cli

import (
	"encoding/csv"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"sshfleet/internal/common"
	"sshfleet/internal/ssh"
)

// -a（代填）的取值处理：来源判定、解析、门控、互斥与长度检查，都在选项/参数合规检查阶段一次做完。
//
// 取值形态与 -f 完全同构：给一个 CSV 文件路径，或直接写同样格式的内联文本
// （命令行里用 `\n` 分行）。解析走 common.ReadCSVRows——与清单同一份实现，
// `#` 注释行、空行、BOM、双引号、变长列的口径全部一致，没有第二套规则。
//
// 列语义是代填自己的：第 1 列是代填内容，第 2 列起是触发词，列数自适应。

// answerLimit 最终下发行的字节上限（写死，不开放配置）。
//
// 依据：整条远端命令行由 sshd 以 `$SHELL -c "<整条>"` 交给内核 execve，是**单个 argv 串**，
// 受 Linux MAX_ARG_STRLEN（PAGE_SIZE × 32 = 131072）约束。这里取 120KB 留出余量。
const answerLimit = 122880

// checkAnswer 校验 -a 的值并解析成代填表（写进 a.Answers）。
// scriptText 是 -s 已读到的脚本文件内容（命令模式传 nil）；长度检查要用它算下发行。
func checkAnswer(a *Args, scriptText []byte) error {
	if a.Answer == "" {
		return nil
	}
	if err := checkAnswerExclusive(a); err != nil {
		return err
	}

	src := resolveSource(a.Answer)
	// 门控：一条代填至少要有两列（内容 + 触发词），所以内联值一定含逗号。
	// 不含逗号又不是已有文件，多半是路径打错了——照 -f 的同一形状报「文件不存在」。
	if !src.IsFile && !strings.Contains(src.Raw, ",") {
		return fmt.Errorf("-a 参数指定的文件不存在：%s\n"+
			"提示：直接写代填内容就用逗号分隔：代填内容,触发词,触发词", src.Raw)
	}

	text, err := answerText(src)
	if err != nil {
		return err
	}
	if src.IsFile {
		a.AnswerFile = src.Path // 归档备份用
	}
	rows, err := common.ReadCSVRows(text)
	if err != nil {
		return answerReadError(src, err)
	}

	entries := make([]ssh.Answer, 0, len(rows))
	for _, row := range rows {
		entry, err := parseAnswerRow(row.Fields)
		if err != nil {
			return answerLineError(src, row, err)
		}
		entries = append(entries, entry)
	}
	if len(entries) == 0 {
		return answerEmptyError(src)
	}

	a.Answers = entries
	if err := validateAnswers(a.Answers, a.Match); err != nil {
		return err
	}
	return checkAnswerLength(a, scriptText)
}

// answerText 按来源取到要解析的文本。
//
// 内联值先把字面 `\n` 补成真实换行——命令行里传不进换行，这是唯一的补齐动作；
// 补完之后与文件内容走完全同一条解析路径。
func answerText(src source) (string, error) {
	if !src.IsFile {
		return common.ExpandEscapedNewlines(src.Raw), nil
	}
	data, err := os.ReadFile(src.Path)
	if err != nil {
		return "", fmt.Errorf("-a 参数指定的文件不可读：%s\n提示：请检查文件是否存在、当前用户有没有读权限", src.Path)
	}
	if common.IsBinaryContent(data) {
		return "", fmt.Errorf("%s 是二进制文件\n提示：代填要写成文本（CSV）", src.Path)
	}
	return string(data), nil
}

// answerReadError CSV 语法错的文案。Go 的原文是行话，用户只需要知道哪一行、
// 引号要成对（与清单侧同口径，见 nodelist/csvread.go）。
func answerReadError(src source, err error) error {
	var parseErr *csv.ParseError
	if !errors.As(err, &parseErr) {
		return fmt.Errorf("读取代填失败：%v", err)
	}
	if src.IsFile {
		return fmt.Errorf("读取代填失败：代填文件 %s 第 %d 行格式读不了——CSV 的双引号要成对出现\n"+
			"提示：值里要用引号时，把里面的引号写成两个", src.Path, parseErr.Line)
	}
	return fmt.Errorf("读取代填失败：第 %d 行格式读不了——CSV 的双引号要成对出现\n"+
		"提示：值里要用引号时，把里面的引号写成两个", parseErr.Line)
}

// answerLineError 某一条代填不合规。行号是文本里的行号——空行与注释行照样数，
// 报出来的号照着文本就能找到。原因另起一行：外层的报错前缀已经是「原因：」。
func answerLineError(src source, row common.CSVRow, err error) error {
	rowText := strings.Join(row.Fields, ",")
	if src.IsFile {
		return fmt.Errorf("代填文件 %s 第 %d 行不合规：%s\n%v", src.Path, row.Line, rowText, err)
	}
	return fmt.Errorf("-a 的值第 %d 行不合规：%s\n%v", row.Line, rowText, err)
}

// answerEmptyError 一份取值里一条代填都没有：报出来，不静默跑空表。
func answerEmptyError(src source) error {
	if src.IsFile {
		return fmt.Errorf("代填文件 %s 里没有一条代填\n"+
			"提示：一行一条，第 1 列是代填内容、第 2 列起是触发词；空行与 # 开头会被跳过", src.Path)
	}
	return fmt.Errorf("-a 的值里没有一条代填\n" +
		"提示：一行一条，第 1 列是代填内容、第 2 列起是触发词；空行与 # 开头会被跳过")
}

// checkAnswerExclusive -a 与几个开关互斥：冲突的是「同一条会话该长什么样」，明确报错并说明原因。
func checkAnswerExclusive(a *Args) error {
	switch {
	case a.NoShell:
		return fmt.Errorf("-a 不能和 --no-shell 一起用\n" +
			"--no-shell 要求命令原样下发，代填要接管会话，同一个会话满足不了两种要求")
	case a.Upload != "":
		return fmt.Errorf("-a 不能和 -u 一起用\n提示：代填是给命令、脚本的交互用的，上传时没有命令在跑")
	case a.Download != "":
		return fmt.Errorf("-a 不能和 -d 一起用\n提示：代填是给命令、脚本的交互用的，下载时没有命令在跑")
	}
	return nil
}

// parseAnswerRow 解析一条代填：第 1 段是代填内容，其余各段是触发词，列数自适应
// （有几个触发词就是几列）。
//
// 第 1 段留空是合法写法：代表只发一个回车。
func parseAnswerRow(fields []string) (ssh.Answer, error) {
	entry := ssh.Answer{Value: fields[0], Triggers: fields[1:]}
	if len(entry.Triggers) == 0 {
		return ssh.Answer{}, fmt.Errorf("只有一列：缺触发词（第 1 列是代填内容，第 2 列起是触发词）")
	}
	for _, trigger := range entry.Triggers {
		if strings.TrimSpace(trigger) == "" {
			return ssh.Answer{}, fmt.Errorf("触发词不能为空")
		}
	}
	return entry, nil
}

// validateAnswers 代填表的口径校验：触发词是唯一的门控——没有它等于无条件盲填。
// 正则模式下顺带预编译触发词：语法不对在本地就拦下，不留到运行期静默不匹配
// （那时用户只会看到「触发词未命中」，而真原因是触发词自己写错了）。
func validateAnswers(answers []ssh.Answer, opts ssh.MatchOptions) error {
	if len(answers) == 0 {
		return fmt.Errorf("-a 参数没有解析出任何一条代填")
	}
	for i, entry := range answers {
		if len(entry.Triggers) == 0 {
			return fmt.Errorf("-a 第 %d 条代填缺少触发词：%s\n提示：触发词是唯一的门控，没有它等于无条件盲填", i+1, entry.Value)
		}
		if err := ssh.ValidateKeywords(entry.Triggers, opts); err != nil {
			return fmt.Errorf("-a 第 %d 条代填：%v", i+1, err)
		}
	}
	return nil
}

// checkAnswerLength 长度检查：判据是最终下发行的字节数，仅 -a 运行执行——
// 普通路径维持 stdin 交付、没有长度上限。超限在本地拦下退出，不放到远端执行时才报错。
// 下发行在这里算好一次、存进 Args.Downlink，执行侧直接取用，不再自己拼（ADR-0010）。
func checkAnswerLength(a *Args, scriptText []byte) error {
	line := ssh.InteractiveCommand(downlinkSpecOf(a, scriptText))
	a.Downlink = line
	if len(line) <= answerLimit {
		return nil
	}
	body := len(a.Command)
	if a.Script != "" {
		body = len(scriptText)
	}
	return fmt.Errorf("-a 参数下发的命令太长：正文 %d 字节，下发 %d 字节，上限 %d 字节\n"+
		"提示：长度受工具限制，请核减脚本内容", body, len(line), answerLimit)
}

// downlinkSpecOf 装配下发行的输入（脚本模式的正文取自脚本文件内容）——**全场唯一一装配点**。
// 长度检查与执行读的都是由它拼成的那一条：量的串 = 实际发的串（ADR-0010）。
//
// 不收代填表与匹配口径：它们不参与下发行的拼装，只在执行期用（见 ssh.InteractiveInput）。
func downlinkSpecOf(a *Args, scriptText []byte) ssh.DownlinkSpec {
	spec := ssh.DownlinkSpec{
		Command:     a.Command,
		Interpreter: a.Interpreter,
		EnvPrefix:   a.EnvPrefix,
		AsRoot:      a.Sudo,
	}
	if a.Script != "" {
		spec.ScriptBody = ScriptMaterialOf(a, scriptText).Body
		spec.ScriptName = filepath.Base(a.Script)
	}
	return spec
}
