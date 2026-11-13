package parse

// 下发行的拼装：交给 SSH 执行的那一条命令行，以及经 stdin 送下去的正文。
//
// 形态逐字照老版 `internal/ssh/command.go`：
//
//	sh -c 'export …; <编码判断> [sudo ]解释器'
//
// 四段各管一件事——外壳只提供「先设环境、再起解释器」的容器；环境段把 `[env]` 导进去；
// 执行身份决定 sudo 加不加；解释器决定命令交给谁。**正文不进命令行**，走 stdin，
// 所以内容不参与 shell 解析，也没有命令行长度上限（`-a` 那一支除外，它必须进命令行）。
//
// 为什么在解析站拼（建站 05 规格第三节）：ADR-0010 的补充要求「参数合规阶段拼一次、存起来，
// 量的串＝发的串」。建站 04 把它放在执行站不是判据，是包归属的连带——老版这个函数长在
// `ssh` 包里，而迁移地图把整个 `ssh` 包判给了执行站。现在搬回来：纸上一格成品串，
// 执行站只发不拼，`-a` 的长度闸门也就量得到真正要发的那条串。
//
// 单点拼装：量的串＝发的串＝日志里那一行。拼法散到两处就会各自漂移。

import (
	"encoding/base64"
	"path/filepath"
	"sort"
	"strings"

	"sshfleet2/internal/contract"
	"sshfleet2/internal/criterion"
)

// sealDownlink 把下发行拼好放进纸里，并把「拼不出来」那一件记成事实。
//
// 拼不出来只有一种情形：`-a` 碰上表外的解释器——没有统一的「一段程序文本」入口，
// 硬拼会用错参数静默不跑。解析站不下结论，只记一条 Problem 交给闸门拒。
func sealDownlink(p *contract.Plan) {
	if p.Run.Answer != "" {
		if _, ok := programTextArg(p.Run.Interpreter); !ok {
			p.Problems = append(p.Problems, contract.Problem{
				Kind:   contract.ProblemInterpreterUnsupported,
				Option: "--answer",
				Raw:    p.Run.Interpreter,
			})
			return
		}
	}
	p.Run.Downlink, p.Run.Stdin = downlinkOf(p)
}

// downlinkOf 拼这一次的下发行与 stdin 正文。**全场唯一的装配点。**
//
// 四支按「这次要干什么」分派：
//   - `--no-shell`：原样下发，不套外壳、不设环境、不加 sudo，正文也不进 stdin；
//   - 给了 `-a`：正文 base64 编进命令行，stdin 整条让给代填；
//   - 脚本：正文走 stdin，shell 家族还要把脚本名补成 `$0`；
//   - 命令：正文走 stdin。
func downlinkOf(p *contract.Plan) (downlink, stdin string) {
	run := p.Run
	if run.NoShell {
		// 使用者在明确要求「原样执行」，不该由工具代他决定环境与身份（ADR-0031）。
		// 它只服务命令模式（闸门已把 `-s`／`-u`／`-d` 挡在外面）。
		if p.Task.Command != nil {
			return p.Task.Command.Text, ""
		}
		return "", ""
	}

	interpreter := run.Interpreter
	if run.Answer != "" {
		body := ""
		scriptName := ""
		switch {
		case p.Task.Script != nil:
			body = p.Task.Script.Body
			scriptName = filepath.Base(p.Task.Script.Path)
		case p.Task.Command != nil:
			body = p.Task.Command.Text
		}
		return interactiveCommand(run, body, scriptName), ""
	}

	if p.Task.Script != nil {
		name := filepath.Base(p.Task.Script.Path)
		inner := envHead(run.Env, run.English) + sudoPrefix(run.Sudo) + scriptInvocation(interpreter, name)
		return "sh -c " + criterion.SingleQuoted(inner), p.Task.Script.Body
	}
	if p.Task.Command != nil {
		inner := envHead(run.Env, run.English) + sudoPrefix(run.Sudo) + interpreter
		return "sh -c " + criterion.SingleQuoted(inner), p.Task.Command.Text
	}
	return "", ""
}

// interactiveCommand 交互分支的下发行：正文 base64 编入命令行，远端 base64 -d 之后
// 作内层解释器的「程序文本参数」。
//
// 为什么不走 stdin：一次会话只有一条 stdin，代填要独占它；两者共用时远端 read 会吃掉
// 脚本体自己的下一行，后半段静默消失且不报错。为什么不落盘：目标机磁盘满是常态，
// 落 /tmp 会因磁盘失败（ADR-0010）。
//
// 「程序文本参数」各家不同（shell／python 是 -c，perl／ruby／node／lua 是 -e，php 是 -r），
// 由 `programTextArg` 按解释器取；表外的解释器在解析站就被记成一条事实、闸门拒，
// 这里兜底用 -c（走不到，兜的是「万一」）。
func interactiveCommand(run contract.RunParams, body, scriptName string) string {
	encoded := base64.StdEncoding.EncodeToString([]byte(body))
	programArg, ok := programTextArg(run.Interpreter)
	if !ok {
		programArg = "-c"
	}
	inner := envHead(run.Env, run.English) + sudoPrefix(run.Sudo) + run.Interpreter +
		" " + programArg + ` "$(printf %s ` + encoded + ` | base64 -d)"`
	// 命令模式没有文件名，不补 $0。
	if scriptName != "" {
		inner += scriptNameArg(run.Interpreter, scriptName)
	}
	return "sh -c " + criterion.SingleQuoted(inner)
}

// sudoPrefix 执行身份那一段。
func sudoPrefix(asRoot bool) string {
	if asRoot {
		return "sudo "
	}
	return ""
}

// envHead 下发行开头的环境段（含尾随空格）：`[env]` 的 export 在前，locale 片段在后。
// export 段为空时只剩片段，免得拼出多余空格。
func envHead(env map[string]string, english bool) string {
	frag := localeFragment(english)
	if prefix := exportPrefix(env); prefix != "" {
		return prefix + " " + frag + " "
	}
	return frag + " "
}

// exportPrefix 把 `[env]` 拼成那段 `export …;`（空配置返回空串）。
//
// 出厂配置只给「一组标准 PATH + 哑终端」两条，理由照老版：非交互拉起命令时 PATH 常只剩
// /usr/bin:/bin，sudo 与 python3 会 command not found；靠登录 shell 读 profile 要求目标机
// 有 bash、认 -l、profile 语法可解析，三样都不保证。TERM=dumb 则抑制彩色与转义序列。
//
// 键名排序：同一份配置每次拼出的串一样，日志与 A/B 比对才不会出现无端差异。
func exportPrefix(env map[string]string) string {
	if len(env) == 0 {
		return ""
	}
	names := make([]string, 0, len(env))
	for name := range env {
		names = append(names, name)
	}
	sort.Strings(names)

	var b strings.Builder
	b.WriteString("export ")
	for i, name := range names {
		if i > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(name)
		b.WriteByte('=')
		b.WriteString(quoteEnvValue(env[name]))
	}
	b.WriteByte(';')
	return b.String()
}

// encodingFragment 下发行里那段「让目标机按字符算」的判断，POSIX sh，不含单引号。
//
// 判据是**白名单**，不是「不等于 UTF-8」：只有目标机当前 charmap 确知是单字节
// （C／POSIX／ANSI_X3.4-1968／US-ASCII），且机器上确实有 C.UTF-8／en_US.UTF-8 可选时，
// 才 export LC_CTYPE。zh_CN.GB18030、GBK、Big5 都是多字节 locale，换掉反而退化——
// 字节流在 C.utf8 下不是合法 UTF-8，`cut -c` 会从「切完整字」变成「切半个字」。
//
// 三条不变式：只动 LC_CTYPE（字符语义与提示语语言是两个轴）；永不降级（白名单外不动、
// 候选不存在不动）；永不告警（外部命令全部 2>/dev/null，不设 LC_ALL）。
//
// 候选用 `set --` 落在位置参数上、不留具名变量：sh 没有 local，用户 `[env]` 里若有同名
// 变量，赋值会连带改掉它导出的值。位置在 `[env]` 之后、解释器之前——判的是最终生效的 locale。
const encodingFragment = `case "$(locale charmap 2>/dev/null)" in C|POSIX|ANSI_X3.4-1968|US-ASCII) set -- $(locale -a 2>/dev/null | grep -Eixm1 "c\.utf-?8|en_us\.utf-?8" 2>/dev/null); [ -n "$1" ] && export LC_CTYPE="$1";; esac;`

// localeFragment 下发行里的 locale 片段：编码判断恒在，语言开关开了才多一句。
//
// 编码部分没有开关（使用者看不见，也不该让他关掉）。语言只留 `english` 这一个开关：
// 真则加一句 `export LC_MESSAGES=C`，目标机上**命令自己**的提示语统一说英文。
//
// 边界要写清：`LC_MESSAGES` 只管得到本地程序的 gettext，改不到远端 `sshd`／`pam` 那种
// **服务端**报文——分类表里那两条中文关键词就是服务端吐的，开关开了它们仍旧是中文，
// 所以中英两套都得留在表里。
func localeFragment(english bool) string {
	if english {
		return encodingFragment + " export LC_MESSAGES=C;"
	}
	return encodingFragment
}

// quoteEnvValue 把环境变量的**值**包成可直接下发的写法：一律双引号包裹，值里的双引号
// 转义成 \"。只做这一件转义。
//
// 双引号拦不住 `$`、`$(…)` 与反引号——那是**故意的**：值的语义就是交目标节点在导入时展开。
// 唯一的例外是值以 `\` 结尾，那个反斜杠会吃掉收尾引号、整条下发串变成语法错；这类值在
// 启动期就被 config 校验拦下，这里不再兜。
func quoteEnvValue(v string) string {
	return `"` + strings.ReplaceAll(v, `"`, `\"`) + `"`
}

// —— 解释器 ——
//
// 表住解析站一处：一行一个名字，带三样机器要的东西（程序文本参数、是不是 shell 家族、
// source /dev/stdin 之前要先关掉的 shell 选项）。
//
// **表里不放句子、也不放显示顺序**：「目前支持：…」那句与名字的先后归 `message`
// （唯一改字处）。两边名字不许漂——解析站有一条用例按集合比对。

// interpreterInfo 解释器登记项。
type interpreterInfo struct {
	name       string // 归一后的标准名，也是查表的 key
	programArg string // 「用一段程序文本启动它」的参数：-c / -e / -r
	shell      bool   // 是否 shell 家族：决定脚本模式下补不补脚本名（$0）
	srcPrelude string // source /dev/stdin 之前要先关掉的 shell 选项（只有 zsh 需要）
}

// interpreterTable 全工具认的解释器。
var interpreterTable = []interpreterInfo{
	{"bash", "-c", true, ""},
	{"sh", "-c", true, ""},
	{"dash", "-c", true, ""},
	{"ksh", "-c", true, ""},
	{"zsh", "-c", true, "setopt no_function_argzero"},
	{"mksh", "-c", true, ""},
	{"ash", "-c", true, ""},
	{"python", "-c", false, ""},
	{"luajit", "-e", false, ""},
	{"lua", "-e", false, ""},
	{"node", "-e", false, ""},
	{"nodejs", "-e", false, ""},
	{"perl", "-e", false, ""},
	{"ruby", "-e", false, ""},
	{"php", "-r", false, ""},
}

// interpreterNames 是表里的名字（无序）：只给对账用例用，不面向使用者。
func interpreterNames() []string {
	out := make([]string, 0, len(interpreterTable))
	for _, it := range interpreterTable {
		out = append(out, it.name)
	}
	return out
}

// interpreterInfoOf 按归一后的标准名查表：`/bin/sh`、`zsh5`、`perl -w`、`php8.1` 都认。
func interpreterInfoOf(interpreter string) (interpreterInfo, bool) {
	name := criterion.InterpreterBaseName(interpreter)
	if name == "" {
		return interpreterInfo{}, false
	}
	for _, it := range interpreterTable {
		if it.name == name {
			return it, true
		}
	}
	return interpreterInfo{}, false
}

// programTextArg 「用一段程序文本启动它」的参数；表外的名字交回 false。
//
// 各家**不一样**：shell 家族与 python 是 `-c`；perl／ruby／node／lua 是 `-e`——
// 它们的 `-c` 是「只做语法检查」，会静默跑空；php 是 `-r`。
func programTextArg(interpreter string) (string, bool) {
	it, ok := interpreterInfoOf(interpreter)
	if !ok {
		return "", false
	}
	return it.programArg, true
}

// isShellFamily 解释器是不是 shell 家族：归一后查表。
func isShellFamily(interpreter string) bool {
	it, ok := interpreterInfoOf(interpreter)
	return ok && it.shell
}

// scriptNameArg 脚本模式补「脚本名」的那一段参数：`<解释器> -c <正文或引导> '<脚本名>'`
// ——shell 的 -c 之后的第一个参数就是本次执行的 $0。
//
// 为什么必须补：脚本正文不落盘，$0 只是解释器名、BASH_SOURCE 更是空的。于是「靠自己的
// 文件名取信息」的脚本什么都取不到（安装包常见：文件名里带目标 IP／站点类型），后面拿这个
// 空值做数值比较会直接报 `[: -eq: unary operator expected`。
//
// 为什么只给 shell 家族补：python3 的 `-c` 之后第一个参数落在 sys.argv[1] 而不是 argv[0]，
// 补了只是往脚本的参数表里塞一个它不认的东西。
func scriptNameArg(interpreter, scriptName string) string {
	if scriptName == "" || !isShellFamily(interpreter) {
		return ""
	}
	return " " + criterion.SingleQuoted(scriptName)
}

// scriptInvocation 脚本模式下解释器自己那条调用串——不含环境前缀、不含 sudo：
//   - shell 家族：`解释器 -c '. /dev/stdin' '脚本名'`（把 stdin 上的脚本体读进来跑）；
//   - 其余（python／perl／ruby／…）：裸解释器——它们从 stdin 直接读脚本。
func scriptInvocation(interpreter, scriptName string) string {
	if arg := scriptNameArg(interpreter, scriptName); arg != "" {
		return interpreter + " -c " + criterion.SingleQuoted(sourceStdinBody(interpreter)) + arg
	}
	return interpreter
}

// sourceStdinBody `-c` 里那段 source /dev/stdin 的程序文本。
//
// 需要补名字时不能只把一个参数挂在解释器后面——解释器读 stdin 时 $0 取自它自己的 argv[0]，
// 挂参数没用。办法是 `-c '. /dev/stdin' '<名字>'`：`.` 把 stdin 上的脚本体读进当前 shell，
// 而 -c 的第一个参数就是 $0。zsh 要单独适配：它的 source 会把 $0 重置成被 source 的文件名
// （FUNCTION_ARGZERO 默认开），所以先把它关掉。
func sourceStdinBody(interpreter string) string {
	body := ". /dev/stdin"
	if it, ok := interpreterInfoOf(interpreter); ok && it.srcPrelude != "" {
		body = it.srcPrelude + "; " + body
	}
	return body
}
