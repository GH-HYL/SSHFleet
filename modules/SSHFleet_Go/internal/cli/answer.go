package cli

import (
	"fmt"
	"os"
	"path"
	"strings"

	"sshfleet/internal/ssh"
)

// -a（代填）的取值处理：来源判定、解析、门控、互斥与长度检查，都在参数合规检查阶段一次做完。
//
// 与 -f 的 FIsInline 同一手法：来源（内联 / 文件）在本阶段判一次、结果存进 Args，
// 执行侧直接用——不两处各判一次。

// answerLimit 最终下发行的字节上限（写死，不开放配置）。
//
// 依据：整条远端命令行由 sshd 以 `$SHELL -c "<整条>"` 交给内核 execve，是**单个 argv 串**，
// 受 Linux MAX_ARG_STRLEN（PAGE_SIZE × 32 = 131072）约束。这里取 120KB 留出余量。
const answerLimit = 122880

// checkAnswer 校验 -a 的值并解析成代填表（写进 a.Answers）。
// scriptText 是 -s 已读到的脚本文件内容（命令模式传 nil）；长度检查要用它算下发行。
func checkAnswer(a *Args, scriptText []byte) error {
	if len(a.Answer) == 0 {
		return nil
	}
	if err := checkAnswerExclusive(a); err != nil {
		return err
	}
	for _, raw := range a.Answer {
		entries, err := readAnswerValue(raw)
		if err != nil {
			return err
		}
		a.Answers = append(a.Answers, entries...)
	}
	if err := validateAnswers(a.Answers, a.Match); err != nil {
		return err
	}
	return checkAnswerLength(a, scriptText)
}

// checkAnswerExclusive -a 与几个开关互斥：冲突的是「同一条会话该长什么样」，明确报错并说明原因。
func checkAnswerExclusive(a *Args) error {
	switch {
	case a.NoBash:
		return fmt.Errorf("-a 不能和 --no-bash 一起用\n" +
			"原因：--no-bash 要求命令原样下发，代填要接管会话（分配终端、正文另走命令行承载），同一个会话满足不了两种要求")
	case a.Upload != "":
		return fmt.Errorf("-a 不能和 -u 一起用\n提示：代填是给命令、脚本的交互用的，上传时没有命令在跑")
	case a.Download != "":
		return fmt.Errorf("-a 不能和 -d 一起用\n提示：代填是给命令、脚本的交互用的，下载时没有命令在跑")
	}
	return nil
}

// readAnswerValue 一条 -a 的值 → 若干代填。来源判定照 -f 的形状判定：
// 先看路径存不存在，不存在再按形态判——含逗号即内联，不含逗号报「文件不存在」。
func readAnswerValue(raw string) ([]ssh.Answer, error) {
	if _, err := os.Stat(raw); err == nil {
		return readAnswerFile(raw)
	}
	if !strings.Contains(raw, ",") {
		return nil, fmt.Errorf("-a 参数指定的文件不存在：%s\n"+
			"提示：直接写代填内容就用逗号分隔：代填内容,触发词,触发词", raw)
	}
	entry, err := parseAnswerLine(raw)
	if err != nil {
		return nil, fmt.Errorf("-a 参数的值不合规：%s\n原因：%v", raw, err)
	}
	return []ssh.Answer{entry}, nil
}

// readAnswerFile 读代填文件：一行一条，与内联同构（第 1 列代填内容、第 2 列起触发词、
// 列数自适应，有几个触发词就几列）；空行与 # 开头跳过；可与内联混用。
//
// 不复用节点清单的读取器：那是按节点为单位的，第 1 列必须是 IP。
func readAnswerFile(file string) ([]ssh.Answer, error) {
	data, err := os.ReadFile(file)
	if err != nil {
		return nil, fmt.Errorf("-a 参数指定的文件不可读：%s\n提示：请检查文件是否存在、当前用户有没有读权限", file)
	}
	// 剥 UTF-8 BOM：Windows 记事本存 CSV 默认带它，留着会让首行的 # 注释判断失效
	// （清单侧同口径，见 nodelist.csvread）。
	text := strings.TrimPrefix(string(data), "\ufeff")
	var out []ssh.Answer
	for i, line := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		entry, err := parseAnswerLine(line)
		if err != nil {
			return nil, fmt.Errorf("代填文件第 %d 行不合规：%s\n原因：%v", i+1, line, err)
		}
		out = append(out, entry)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("-a 参数指定的文件里没有一条代填：%s\n提示：一行一条，写法与内联相同：代填内容,触发词,触发词", file)
	}
	return out, nil
}

// parseAnswerLine 解析一条代填：逗号分隔，第 1 段是代填内容，其余各段是触发词。
//
// 「代填内容与触发词都不能含逗号」不必单独校验——逗号就是分隔符，它一定被拆走；
// 这条规则是给用户看的（帮助与使用手册）：写了逗号会**被当成多个触发词**，而不是报错。
// 第 1 段留空是合法写法：代表只发一个回车。
func parseAnswerLine(line string) (ssh.Answer, error) {
	parts := strings.Split(line, ",")
	entry := ssh.Answer{Value: parts[0], Triggers: parts[1:]}
	if len(entry.Triggers) == 0 {
		return ssh.Answer{}, fmt.Errorf("缺少触发词（代填内容与触发词之间用逗号分隔）")
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
func checkAnswerLength(a *Args, scriptText []byte) error {
	line := ssh.InteractiveCommand(answerInputOf(a, scriptText))
	if len(line) <= answerLimit {
		return nil
	}
	return fmt.Errorf("-a 参数下发的命令太长：%d 字节，上限 %d 字节\n"+
		"原因：正文要 base64 编入命令行，受 Linux 单个参数的长度上限约束\n"+
		"提示：把大块内容改走上传模式，或拆成多条短命令", len(line), answerLimit)
}

// answerInputOf 由命令行参数拼交互分支的入场值（正文取自脚本文件内容）。
// 与 batch 侧拼的是同一个结构：本包只用来算下发行长度，不参与执行。
func answerInputOf(a *Args, scriptText []byte) ssh.InteractiveInput {
	in := ssh.InteractiveInput{
		Command: a.Command,
		AsRoot:  a.Sudo,
		Answers: a.Answers,
		Match:   a.Match,
	}
	if a.Script != "" {
		in.ScriptBody = ssh.ScriptBodyOf(scriptText)
		in.Interpreter = "bash"
		if path.Ext(a.Script) == ".py" {
			in.Interpreter = "python3"
		}
	}
	return in
}
