package ssh

import (
	"fmt"
	"strings"
)

// 语言变量前缀：收进登录 shell 内部，export 确保对内部所有命令及子进程生效
// （C.UTF-8 是 POSIX 标准，所有 Linux 发行版内置支持）。
const envPrefix = "export LC_ALL=en_US.UTF-8 LANG=en_US.UTF-8;"

// 执行模式（只用到这三种取值；由调用方从命令行参数解读后传入）。
const (
	modeSudo = "sudo"
)

// BuildCommand 构建下发的命令与 stdin 内容（spec 实现途径 1）：
// 命令/脚本内容经 session.Stdin 直喂，命令行只剩 `bash -lc '<env; sudo bash|bash>'`，
// 不再走 base64 通道——命令行不再有长度上限风险，也不需要引号转义套娃。
//
// 只收命令构建真正需要的几个值，不收整份命令行参数——本包是传输层，
// 不该认识参数载体（否则想单独测它得先凑齐一整套参数）。
//
// 参数：
//   - command:     命令模式下的命令原文（脚本模式传空）
//   - scriptBody:  脚本模式下的脚本内容（命令模式传空）
//   - interpreter: 脚本解释器（"bash" / "python3"；命令模式传空）
//   - noBash:      --nobash：命令模式专用，原样下发
//   - asRoot:      -m sudo：以 root 身份执行
//
// 返回 (下发命令, stdin 内容)：stdin 为空表示不喂输入（--nobash 命令模式）。
func BuildCommand(command, scriptBody, interpreter string, noBash, asRoot bool) (string, string) {
	// --nobash 为命令模式专用：原样下发，不套登录 shell、不喂 stdin
	if noBash && command != "" {
		return command, ""
	}

	// 命令模式与脚本模式共用同一个登录 shell 包裹（唯一差别是内层解释器）。
	// 内层串由 loginInner 出，与 DescribeCommand 打印的「下发行」同源。
	isScript := scriptBody != ""
	if isScript {
		return "bash -lc " + shellQuote(loginInner(true, interpreter, asRoot)), scriptBody
	}
	return "bash -lc " + shellQuote(loginInner(false, "", asRoot)), command
}

// shellQuote 单引号包裹（对位旧 shlex.quote 的单引号路径）：内部单引号按 '\” 转义，
// 保证命令作为 bash -lc 的单个参数传递时不发生二次解析。
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// QuoteForShell 把一段**参数原文**还原成等价的命令行写法：需要时以单引号包裹
// （内部单引号按 '\” 转义），不需要时原样返回。
//
// 用途仅限「打印给人看的命令行」——日志与 report.txt 里的执行命令。
// 不下发、不参与任何解析，故不影响执行行为。
//
// 为什么需要它：用户敲的是 `-c 'who -b'`，argv 递过来时引号已被本机 shell 剥掉，
// 直接平铺打印会变成 `-c who -b`——那是一条会把 `-b` 拆成多余参数的命令，
// 事后照抄重跑就跑不起来了。判定用「会不会被 shell 二次拆词」，而不是「原样里有没有
// 空格」：`-f a,b` 这类含逗号的参数、`-c ls` 这类纯单字参数本就不需要引号，加了反而失真。
func QuoteForShell(s string) string {
	if s == "" {
		return `''`
	}
	if !needsQuoting(s) {
		return s
	}
	return shellQuote(s)
}

// shellSafeChars 不被 shell 二次解释的字符集：字母数字与 `_-./:=+,@%^`。
// 逗号在内——`-f` 的内联清单是逗号分隔值，引号包裹会让报告里的写法失真。
const shellSafeChars = "_-./:=+,@%^"

// needsQuoting 判断一段参数被 shell 二次拆词 / 展开的可能性。
// 空白一律要引号（拆词）；`~` `*` `?` `$` 等由默认分支覆盖（展开）。
func needsQuoting(s string) bool {
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case strings.ContainsRune(shellSafeChars, r):
		default:
			return true
		}
	}
	return false
}

// DescribeCommand 交代一次下发「包成了什么」——供日志与报告打印，不参与下发。
//
// 重构前（旧 Python builder.py）这里有一整段「完整命令拼接完成 / 原始命令 / 处理方式 /
// 最终命令」的日志，重构后连同 base64 通道一起没了：命令怎么被包进登录 shell、
// 内容走哪条通道，事后全查不到。本函数把这层黑箱重新说清楚。
//
// 下发行由本包自己拼（与 BuildCommand 同源），调用方只给业务侧的几项——
// bash -lc 的形态是本包的格式，散到调用方就会两处各写一份、各自漂移。
//
// 返回多行文本，调用方逐行写日志即可。
func DescribeCommand(a DescribeInput) string {
	identity := a.Identity
	if identity == "" {
		identity = "登录用户"
	}

	// --nobash 为命令模式专用：原样下发，不套登录 shell、不喂 stdin
	if a.NoBash && a.Command != "" {
		return strings.Join([]string{
			"处理方式：--nobash 原样下发，不套登录 shell、不经 stdin 通道",
			"下发命令：" + a.Command,
		}, "\n")
	}

	inner := loginInner(a.ScriptBody != "", a.Interpreter, a.AsRoot)
	loginLine := "bash -lc " + shellQuote(inner)

	if a.ScriptBody != "" {
		lines := []string{
			"处理方式：脚本内容经 stdin 直喂（写进会话输入通道，不进命令行）",
			"执行身份：" + identity + "，解释器：" + a.Interpreter,
			"下发行（命令行）：" + loginLine,
			"下发内容（经 stdin）：脚本 " + a.ScriptPath,
		}
		return strings.Join(append(lines, scriptPreview(a.ScriptBody)...), "\n")
	}

	if a.Command != "" {
		return strings.Join([]string{
			"处理方式：命令文本经 stdin 直喂（不进命令行），由内层 shell 读取执行",
			"执行身份：" + identity,
			"下发行（命令行）：" + loginLine,
			"下发内容（经 stdin）：" + a.Command,
		}, "\n")
	}
	return ""
}

// loginInner 拼登录 shell 的内层命令（env 前缀 + [sudo ]解释器）。
// BuildCommand 与 DescribeCommand 共用一处，保证「打印的下发行」与「真下发的行」同源。
func loginInner(isScript bool, interpreter string, asRoot bool) string {
	sudo := ""
	if asRoot {
		sudo = modeSudo + " "
	}
	if isScript {
		return envPrefix + " " + sudo + interpreter
	}
	return envPrefix + " " + sudo + "bash"
}

// DescribeInput DescribeCommand 的入参：只收打印真正需要的几项。
type DescribeInput struct {
	Command     string // 命令模式：命令原文
	ScriptPath  string // 脚本模式：脚本文件路径（正文太长，抬头里报路径）
	ScriptBody  string // 脚本模式：脚本正文（只打印前几行）
	Interpreter string // 脚本模式：bash / python3
	Identity    string // 执行身份文案（空则按「登录用户」）
	NoBash      bool   // --nobash 命令模式
	AsRoot      bool   // -m sudo（拼下发行用）
}

// maxPreviewLines 脚本正文的打印上限：脚本可能有几百行，日志只要交代形态。
const maxPreviewLines = 10

// scriptPreview 脚本正文的前若干行（超出只报剩余行数）。
func scriptPreview(body string) []string {
	lines := strings.Split(strings.ReplaceAll(body, "\r\n", "\n"), "\n")
	head := lines
	rest := 0
	if len(lines) > maxPreviewLines {
		head, rest = lines[:maxPreviewLines], len(lines)-maxPreviewLines
	}
	out := make([]string, 0, len(head)+1)
	for _, ln := range head {
		out = append(out, "  | "+ln)
	}
	if rest > 0 {
		out = append(out, fmt.Sprintf("  ……其余 %d 行省略", rest))
	}
	return out
}
