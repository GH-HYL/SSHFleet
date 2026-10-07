package ssh

import (
	"path"
	"strings"
)

// envPrefix 下发行开头要导入的环境：统一 locale + 一组标准 PATH。收在外壳内部，
// export 确保对内层解释器及其所有子进程生效。
//
// locale：SSH 非交互会话拿到的是目标机默认的 locale，各机器不一样——有的 C、有的
// POSIX、有的跟随发行版设置，于是同一条命令在不同机器上输出的字符编码与排序规则都
// 不同，中文与 UTF-8 内容会变成乱码或按字节处理。强制成同一个 UTF-8，让「同一批节点
// 上跑同一条命令」的输出形态保持一致、可比对。（en_US.UTF-8 比 C.UTF-8 更稳。）
//
// PATH：非交互拉起命令时 PATH 常只剩 /usr/bin:/bin，sudo（/usr/sbin、/usr/local/bin）
// 与 python3（/usr/local/bin、/opt/…）会 command not found。这里直接写死一组标准目录，
// 不再靠登录 shell 去读 /etc/profile——那样要求目标机有 bash、shell 认 -l、profile 还得
// 是它能解析的语法，三样都不保证。PATH 里写不存在的目录不会报错（查找时静默跳过），
// 不影响命令输出；解释器在非常规位置时，由用户在配置里写绝对路径。
const envPrefix = "export LC_ALL=en_US.UTF-8 LANG=en_US.UTF-8 " +
	"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin;"

// 执行模式（只用到这三种取值；由调用方从命令行参数解读后传入）。
const (
	modeSudo = "sudo"
)

// BuildCommand 构建下发的命令与 stdin 内容（spec 实现途径 1）：
// 命令/脚本内容经 session.Stdin 直喂，命令行只剩 `sh -c '<env; [sudo ]解释器>'`，
// 不再走 base64 通道——命令行不再有长度上限风险，也不需要引号转义套娃。
//
// 只收命令构建真正需要的几个值，不收整份命令行参数——本包是传输层，
// 不该认识参数载体（否则想单独测它得先凑齐一整套参数）。
//
// # 为什么要套一层 sh -c
//
// 外壳只干一件事：**在启动解释器之前把环境设好**（locale + PATH，见 envPrefix）。
// 目标机用非交互方式拉起命令时环境往往极简（PATH 只剩 /usr/bin:/bin），
// `sudo` 与解释器都会 `command not found`——这是实际踩过的 bug，跟命令本身对不对无关。
//
// 为什么是 `sh -c` 而不是早先的 `bash -lc`：`bash -lc` 靠"登录 shell 读 /etc/profile"
// 补 PATH，这要求目标机有 bash、shell 认 `-l`（`-l` 不是 POSIX 强制）、profile 还得是
// 它能解析的语法——三样都不保证。现在改成本地显式设 PATH，不读 profile，外壳只剩
// `sh -c`：`/bin/sh` 是 POSIX 强制存在的，到处都有（2026-10-07 定）。
//
// # 各行为分别为了什么
//
//   - `sh -c`：提供"先设环境、再起解释器"的容器；不读 profile、不依赖 bash。
//   - `export LC_ALL/LANG/PATH`：统一目标机环境（见 envPrefix 的说明）。
//   - stdin 直喂内容：命令原文 / 脚本正文不进命令行，只经标准输入送下去。
//     好处是内容不参与 shell 解析、没有引号转义套娃、也没有命令行长度上限。
//   - 内层 `[sudo ]解释器`：sudo 时提权的是**解释器本身**
//     （`sudo bash` 而非 `sudo` 单独一条命令），保证命令/脚本整体以 root 跑；
//     解释器由调用方给定——命令模式传命令解释器，脚本模式传脚本解释器。
//   - 脚本模式补回脚本名（$0）：正文不落盘，但「我叫什么」这件事要交回去——
//     见 scriptNameArg 的说明。
//   - `--no-shell` 时全部绕开：用户在明确要求「原样下发」，此时不该由工具
//     代他决定环境与身份。
//
// 参数：
//   - command:     命令模式下的命令原文（脚本模式传空）
//   - scriptBody:  脚本模式下的脚本内容（命令模式传空）
//   - scriptName:  脚本模式下的脚本文件名（下发行把它交回去当 $0；命令模式传空）
//   - interpreter: 解释器（命令模式传命令解释器，脚本模式传脚本解释器）
//   - noShell:      --no-shell：命令模式专用，原样下发
//   - asRoot:      -m sudo：以 root 身份执行
//
// 返回 (下发命令, stdin 内容)：stdin 为空表示不喂输入（--no-shell 命令模式）。
func BuildCommand(command, scriptBody, scriptName, interpreter string, noShell, asRoot bool) (string, string) {
	// --no-shell 为命令模式专用：原样下发，不套外壳、不喂 stdin
	if noShell && command != "" {
		return command, ""
	}

	// 命令模式与脚本模式共用同一个外壳（唯一差别是内层解释器与要不要补脚本名）。
	// 内层串与 DescribeCommand 打印的「下发行」同源。
	if scriptBody != "" {
		return "sh -c " + shellQuote(scriptStdinInner(interpreter, scriptName, asRoot)), scriptBody
	}
	return "sh -c " + shellQuote(innerCommand(interpreter, asRoot)), command
}

// escapeSingleQuotes 把一段文本里的单引号转义成 '\”。
//
// 转义规则全工具只此一份：shellQuote（整包成单引号）与 sftp.escapeShellArg
// （嵌入既有单引号对）都调它。两个函数仍然分开——一个整包、一个嵌入，语义不同；
// 要消掉的是这条**写了两遍**的规则本身，以及"靠注释提醒人保持同口径"的隐患。
func escapeSingleQuotes(s string) string {
	return strings.ReplaceAll(s, "'", `'\''`)
}

// shellQuote 单引号包裹（对位旧 shlex.quote 的单引号路径）：内部单引号按 '\” 转义，
// 保证命令作为 sh -c 的单个参数传递时不发生二次解析。
func shellQuote(s string) string {
	return "'" + escapeSingleQuotes(s) + "'"
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

// DescribeCommand 交代「交给 SSH 执行的命令」是什么——供日志打印，不参与下发。
//
// 命令与脚本内容都从 stdin 送，命令行里只剩固定形态的外壳，所以「内容是什么」
// 本身在日志里是看不见的，必须显式交代。这里不描述包装过程（不含「原始 / 最终」这类
// 前后对照），只给两个实际发生的事实：命令行发的是什么、stdin 送的是什么。
//
// 下发行由本包自己拼（与 BuildCommand 同源），调用方只给业务侧的几项——
// `sh -c` 的形态是本包的格式，散到调用方就会两处各写一份、各自漂移。
func DescribeCommand(a DescribeInput) string {
	// --no-shell：不套外壳、不经 stdin 通道，内容直接就是命令行
	if a.NoShell && a.Command != "" {
		return "交给 SSH 执行：\n" +
			"  命令行： " + a.Command + "\n" +
			"  说明：   --no-shell 原样执行，不套外壳、不经 stdin"
	}

	inner := innerCommand(a.Interpreter, a.AsRoot)
	if a.ScriptBody != "" {
		inner = scriptStdinInner(a.Interpreter, a.ScriptName, a.AsRoot)
	}
	downLine := "sh -c " + shellQuote(inner)

	if a.ScriptBody != "" {
		return "交给 SSH 执行：\n" +
			"  命令行： " + downLine + "\n" +
			"  stdin：  脚本 " + a.ScriptPath + " 的内容（" + a.Interpreter + " 解释）\n" +
			"  说明：   先导入 " + envNote() + "，再以 " + interpreterWho(a.AsRoot) + " 执行 stdin 送来的脚本"
	}

	if a.Command != "" {
		return "交给 SSH 执行：\n" +
			"  命令行： " + downLine + "\n" +
			"  stdin：  " + a.Command + "\n" +
			"  说明：   先导入 " + envNote() + "，再把 stdin 送来的命令交给 " + shellWho(a.Interpreter, a.AsRoot) + " 执行"
	}
	return ""
}

// envNote 命令行里导入的环境变量（对位 envPrefix 的内容，不重复写一遍字面量）。
func envNote() string {
	return "LC_ALL / LANG（UTF-8）、PATH（标准目录）"
}

// shellWho 执行命令的解释器（含提权说明）。
func shellWho(interpreter string, asRoot bool) string {
	if asRoot {
		return "root 身份的 " + interpreter + "（sudo）"
	}
	return "登录用户的 " + interpreter
}

// interpreterWho 执行脚本的解释器（含提权说明）。
func interpreterWho(asRoot bool) string {
	if asRoot {
		return "root 身份"
	}
	return "登录用户身份"
}

// innerCommand 拼外壳的内层命令（env 前缀 + [sudo ]解释器）。
// 命令模式与脚本模式都走它：命令模式传命令解释器，脚本模式传脚本解释器；正文走 stdin
// 的脚本模式另走 scriptStdinInner（多一步把脚本名补成 $0）。下发行由本包单点拼，调用方不重复。
func innerCommand(interpreter string, asRoot bool) string {
	sudo := ""
	if asRoot {
		sudo = modeSudo + " "
	}
	return envPrefix + " " + sudo + interpreter
}

// scriptNameArg 脚本模式补「脚本名」的那一段参数：`<解释器> -c <正文或引导> '<脚本名>'`
// ——shell 的 -c 之后的第一个参数就是本次执行的 $0。
//
// 为什么必须补：脚本模式的正文不是从磁盘上的文件跑的（无 -a 经 stdin 喂给解释器，
// 有 -a 则 base64 编进命令行），两种形态下 $0 都只是解释器名、BASH_SOURCE 更是空的。
// 于是「靠自己的文件名取信息」的脚本（安装包常见：文件名里带目标 IP / 站点类型）
// 什么都取不到，后面拿这个空值做数值比较会直接报
// `[: -eq: unary operator expected`，脚本随即走进「系统不支持」的分支
// （2026-10-07 真机复现：文件名带 IP 的 .sh 在工具下取不到 IP）。
// 把名字交回去，这段逻辑就与「把脚本落到磁盘再执行」一致，而正文依旧不落盘。
//
// 为什么只给 shell 家族补：python3 的 `-c` 之后第一个参数落在 sys.argv[1] 而不是 argv[0]，
// 补了只是往脚本的参数表里塞一个它不认的东西，故 python3 维持原样。shell 家族由
// isShellFamily 判定——这样把 .sh 配成 sh / mksh 也照样补回名字。
// 名字里的括号等特殊字符由 shellQuote 兜住（"(1.2.3.4).sh" 这类文件名必然带括号）。
func scriptNameArg(interpreter, scriptName string) string {
	if scriptName == "" || !isShellFamily(interpreter) {
		return ""
	}
	return " " + shellQuote(scriptName)
}

// shellFamilyNames 需要补脚本名（$0）的 shell 家族：它们支持
// `<解释器> -c '<正文>' '<名字>'` 这种"名字作 $0"的形态。
// mksh / ash 与 bash 同为 POSIX shell，`-c` 后第一个参数同样落到 $0。
var shellFamilyNames = map[string]bool{
	"bash": true, "sh": true, "dash": true, "ksh": true, "zsh": true, "mksh": true, "ash": true,
}

// isShellFamily 解释器是不是 shell 家族：经 InterpreterBaseName 归一后查表，
// `bash` / `sh` / `/bin/sh` / `zsh5` / `ksh93` 都算，`python3` / `perl` 不算；
// 带参数（如 `perl -w`）时只看第一个词。
func isShellFamily(interpreter string) bool {
	return shellFamilyNames[InterpreterBaseName(interpreter)]
}

// programTextArgTable「用一段程序文本启动解释器」的参数：语言名（小写、去版本号）→ 参数。
//
// 各家**不一样**（2026-10-07 实测确认）：
//   - shell 家族 / python：`-c`（POSIX sh 与 CPython 的标准用法）
//   - perl / ruby / node / lua：`-e`——它们的 `-c` 是"只做语法检查"，会静默跑空
//   - php：`-r`
//
// 表外（tclsh、deno、awk…）没有统一的"一段程序文本"入口，`-a` 不支持——在参数合规检查阶段拦下。
var programTextArgTable = map[string]string{
	"ash":    "-c",
	"bash":   "-c",
	"dash":   "-c",
	"ksh":    "-c",
	"mksh":   "-c",
	"sh":     "-c",
	"zsh":    "-c",
	"python": "-c",
	"luajit": "-e",
	"lua":    "-e",
	"node":   "-e",
	"nodejs": "-e",
	"perl":   "-e",
	"ruby":   "-e",
	"php":    "-r",
}

// InterpreterBaseName 把配置里的解释器取值归一成「认名用的标准名」：
// 取第一个词（带参数如 `perl -w` 时只看第一个词）、去目录、转小写、去尾部版本号
// （/usr/bin/python3 → python、php8.1 → php、zsh5 → zsh）。
//
// 这套剥法全工具只有这一份（2026-10-08 审计裁决）：-a 的支持范围、--no-shell 的
// 互斥、$0 补名的家族判定都认它。此前剥法写了两遍、其中家族判定那遍不剥，
// 同一个名字得出两个结果——zsh5 在 -a 检查里认得、在家族判定里认不出，
// $0 静默失效。空值返回空串。
func InterpreterBaseName(interpreter string) string {
	fields := strings.Fields(interpreter)
	if len(fields) == 0 {
		return ""
	}
	return strings.TrimRight(strings.ToLower(path.Base(fields[0])), "0123456789.")
}

// ProgramTextArg 返回「用一段程序文本启动该解释器」的参数（-c / -e / -r）与它是否在支持范围内。
// 解释器名允许带目录与版本号（/usr/bin/python3、php8.1），由 InterpreterBaseName 归一后查表。
//
// 表外返回 ("", false)。-a 的合规检查据此拦下；拼装侧（InteractiveCommand）拿到的只会是表内的解释器。
func ProgramTextArg(interpreter string) (string, bool) {
	arg, ok := programTextArgTable[InterpreterBaseName(interpreter)]
	return arg, ok
}

// scriptStdinInner 正文走 stdin 那条路（无 -a）的内层命令。
//
// 需要补名字时不能只把一个参数挂在后面——解释器读 stdin 时 $0 取自它自己的 argv[0]，
// 挂参数没用（`bash -s <名字>` 只会把名字当成位置参数 $1）。办法是 `-c '. /dev/stdin' '<名字>'`：
// `.`（source）把 stdin 上的脚本体读进当前 shell，而 -c 的第一个参数就是 $0，名字于是还原。
// 正文始终不经命令行，长度上限那件事没有被破坏。
//
// 为什么不用 `exec -a "$0" <解释器>`：那招只有 bash/ksh 系认，dash/ash 的 exec 不认 -a
// （实测 `dash -c 'exec -a "$0" dash'` 报 `exec: -a: not found`），而 `. /dev/stdin` 各家都认。
// 代价：`.` 是"在当前 shell 里执行"，脚本顶层 `return` 会静默返回（当文件跑会报错），
// `$BASH_SOURCE` 会变成 `/dev/stdin`（原招是空）——对本工具的场景（从自身文件名取信息）无影响。
//
// zsh 要单独适配：zsh 的 source 会把 $0 重置成被 source 的文件名（FUNCTION_ARGZERO
// 默认开），补回的名字会被 /dev/stdin 盖掉（2026-10-08 实测）。bash / dash 的 source
// 不动 $0，无此行为。见 sourceStdinBody。
func scriptStdinInner(interpreter, scriptName string, asRoot bool) string {
	sudo := ""
	if asRoot {
		sudo = modeSudo + " "
	}
	if arg := scriptNameArg(interpreter, scriptName); arg != "" {
		return envPrefix + " " + sudo + interpreter + " -c " + shellQuote(sourceStdinBody(interpreter)) + arg
	}
	return envPrefix + " " + sudo + interpreter
}

// sourceStdinBody `-c` 里那段 source /dev/stdin 的程序文本。zsh 先关掉
// FUNCTION_ARGZERO 再 source，$0 才保持 -c 参数给的名字（2026-10-08 实测
// `setopt no_function_argzero; . /dev/stdin` → probe.sh）；其余家族原样 source。
func sourceStdinBody(interpreter string) string {
	body := ". /dev/stdin"
	if InterpreterBaseName(interpreter) == "zsh" {
		body = "setopt no_function_argzero; " + body
	}
	return body
}

// DescribeInput DescribeCommand 的入参：只收打印真正需要的几项。
type DescribeInput struct {
	Command     string // 命令模式：命令原文
	ScriptPath  string // 脚本模式：脚本文件路径
	ScriptBody  string // 脚本模式：脚本正文（非空即判定为脚本模式）
	ScriptName  string // 脚本模式：脚本文件名（下发行把它交回去当 $0）
	Interpreter string // 解释器：命令模式是命令解释器，脚本模式是脚本解释器
	NoShell     bool   // --no-shell 命令模式
	AsRoot      bool   // -m sudo（拼下发行用）
}
