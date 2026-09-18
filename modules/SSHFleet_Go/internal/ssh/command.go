package ssh

import (
	"strings"
)

// 语言变量前缀：收进登录 shell 内部，export 确保对内部所有命令及子进程生效。
//
// 为什么必须显式 export：SSH 非交互会话拿到的是目标机默认的 locale，各机器不一样
// ——有的 C、有的 POSIX、有的跟随发行版设置，于是同一条命令在不同机器上输出的
// 字符编码与排序规则都不同，中文与 UTF-8 内容会变成乱码或按字节处理。这里强制成
// 同一个 UTF-8，让「同一批节点上跑同一条命令」的输出形态保持一致、可比对。
// （en_US.UTF-8 是各发行版普遍内置的 locale，取它比 C.UTF-8 更稳。）
//
// 注意这一层与登录 shell 是**两件独立的事**：即使哪天不套 bash -lc 了，
// 这个 export 仍然要保留——它解决的是「目标环境 locale 不一致」，
// 不是「命令找不到」。
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
// # 为什么要套一层 bash -lc
//
// 这不是预防性的防御，是**修过一个实际踩到的 bug**：
//
// 目标机用非交互方式拉起命令时，PATH 往往只剩 `/usr/bin:/bin` 这类极简值——
// 用户 `.bashrc` 里那些补 PATH 的语句只在交互分支里跑，非交互根本不执行。
// 结果是 `sudo`（常在 `/usr/sbin`、`/usr/local/bin`）和 `python3`
// （常在 `/usr/local/bin`、`/opt/...`）找不到，报 `command not found`，
// 而这跟命令本身对不对毫无关系。
//
// `bash -lc` 起的是**登录 shell**，会读 `/etc/profile` 与 `~/.bash_profile`／
// `~/.profile`，PATH 因此补全，`sudo`／解释器都能被定位到——登录 shell 在完整
// 环境里执行，没有任何一个组件裸露在极简 PATH 下。这是它唯一的存在理由。
//
// # 各行为分别为了什么
//
//   - `bash -lc`：定位 PATH（上面这条）。没有它，目标机的 PATH 差异会直接
//     让命令跑不起来。
//   - `export LC_ALL/LANG`：统一目标机 locale（见 envPrefix 的说明）。
//     与 PATH 是两回事，即使去掉登录 shell 也该保留。
//   - stdin 直喂内容：命令原文 / 脚本正文不进命令行，只经标准输入送下去。
//     好处是内容不参与 shell 解析、没有引号转义套娃、也没有命令行长度上限。
//   - 内层 `[sudo ]bash|python3`：sudo 时提权的是**解释器本身**
//     （`sudo bash` 而非 `sudo` 单独一条命令），保证命令/脚本整体以 root 跑；
//     脚本模式按扩展名换 python3。
//   - `--nobash` 时全部绕开：用户在明确要求「原样下发」，此时不该由工具
//     代他决定环境与身份。
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

// DescribeCommand 交代「交给 SSH 执行的命令」是什么——供日志打印，不参与下发。
//
// 命令与脚本内容都从 stdin 送，命令行里只剩固定形态的登录 shell，所以「内容是什么」
// 本身在日志里是看不见的，必须显式交代。这里不描述包装过程（不含「原始 / 最终」这类
// 前后对照），只给两个实际发生的事实：命令行发的是什么、stdin 送的是什么。
//
// 下发行由本包自己拼（与 BuildCommand 同源），调用方只给业务侧的几项——
// bash -lc 的形态是本包的格式，散到调用方就会两处各写一份、各自漂移。
func DescribeCommand(a DescribeInput) string {
	// --nobash：不套登录 shell、不经 stdin 通道，内容直接就是命令行
	if a.NoBash && a.Command != "" {
		return "交给 SSH 执行：\n" +
			"  命令行： " + a.Command + "\n" +
			"  说明：   --nobash 原样执行，不套登录 shell、不经 stdin"
	}

	loginLine := "bash -lc " + shellQuote(loginInner(a.ScriptBody != "", a.Interpreter, a.AsRoot))

	if a.ScriptBody != "" {
		return "交给 SSH 执行：\n" +
			"  命令行： " + loginLine + "\n" +
			"  stdin：  脚本 " + a.ScriptPath + " 的内容（" + a.Interpreter + " 解释）\n" +
			"  说明：   先导入 " + envNote() + "，再以 " + interpreterWho(a.AsRoot) + " 执行 stdin 送来的脚本"
	}

	if a.Command != "" {
		return "交给 SSH 执行：\n" +
			"  命令行： " + loginLine + "\n" +
			"  stdin：  " + a.Command + "\n" +
			"  说明：   先导入 " + envNote() + "，再把 stdin 送来的命令交给 " + shellWho(a.AsRoot) + " 执行"
	}
	return ""
}

// envNote 命令行里导入的环境变量（对位 envPrefix 的内容，不重复写一遍字面量）。
func envNote() string {
	return "LC_ALL / LANG（UTF-8）"
}

// shellWho 执行命令的 shell（含提权说明）。
func shellWho(asRoot bool) string {
	if asRoot {
		return "root 身份的 bash（sudo）"
	}
	return "登录用户的 bash"
}

// interpreterWho 执行脚本的解释器（含提权说明）。
func interpreterWho(asRoot bool) string {
	if asRoot {
		return "root 身份"
	}
	return "登录用户身份"
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
	ScriptPath  string // 脚本模式：脚本文件路径
	ScriptBody  string // 脚本模式：脚本正文（非空即判定为脚本模式）
	Interpreter string // 脚本模式：bash / python3
	NoBash      bool   // --nobash 命令模式
	AsRoot      bool   // -m sudo（拼下发行用）
}
