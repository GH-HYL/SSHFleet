package execute

// 下发行：交给 SSH 执行的那一条命令行，以及经 stdin 送下去的正文。
//
// 形态逐字照老版 `internal/ssh/command.go`：
//
//	sh -c 'export …; <编码判断> [sudo ]解释器'
//
// 四段各管一件事——外壳只提供「先设环境、再起解释器」的容器；环境段把 `[env]` 导进去；
// 执行身份决定 sudo 加不加；解释器决定命令交给谁。**正文不进命令行**，走 stdin，
// 所以内容不参与 shell 解析，也没有命令行长度上限。
//
// 单点拼装：量的串＝发的串＝日志里那一行。拼法散到两处就会各自漂移，所以解析站只把
// 配置的原值带上纸（`RunParams.Interpreter`、`RunParams.Env`），成形全在这里。

import (
	"sort"
	"strings"

	"sshfleet2/internal/contract"
)

// downCommand 拼这一次要下发的命令行与 stdin 正文。
//
// `--no-shell`：命令原样下发，不套外壳、不设环境、也不加 sudo——使用者在明确要求
// 「原样执行」，不该由工具代他决定环境与身份（ADR-0031）。它只服务命令模式。
func downCommand(task contract.CommandTask, run contract.RunParams) (command, stdin string) {
	if run.NoShell {
		return task.Text, ""
	}
	inner := envHead(run.Env, run.English) + sudoPrefix(run.Sudo) + run.Interpreter
	return "sh -c " + shellQuote(inner), task.Text
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

// shellQuote 单引号包裹：内部单引号按 '\” 转义，保证整段作为 `sh -c` 的单个参数
// 传递时不发生二次解析。
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
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
