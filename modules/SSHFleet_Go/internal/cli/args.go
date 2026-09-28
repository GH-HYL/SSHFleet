// Package cli 承载主干第 3 步（解析命令行）与第 5 步的合规检查部分。
//
// 平级选项，不引子命令、不引 cobra；互斥手写校验、提示文案自撰。
package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"golang.org/x/term"

	"sshfleet/internal/common"
	"sshfleet/internal/config"
	"sshfleet/internal/ssh"

	"github.com/spf13/pflag"
)

// ErrHelp 表示已打印帮助、应以 0 退出（无任何参数或 -h/-–help）。
var ErrHelp = errors.New("help printed")

// Args 是主干各步骤共用的命令行参数载体（纯数据）。
type Args struct {
	Command        string // -c
	Script         string // -s
	Upload         string // -u
	Download       string // -d
	CsvFile        string // -f
	Path           string // -p
	Sudo           bool   // 本次生效的执行身份：true=root，false=登录用户
	Timeout        int    // -t（缺省时按模式取配置默认）
	ConnectTimeout int    // -T
	Number         int    // -n
	Remark         string // -r
	Key            bool   // -k：写了就用密钥登录（私钥取清单第 5 列，其次配置 key）
	NoBash         bool   // --no-bash
	Disinteractive bool   // --yes
	GenKey         bool   // --gen-key
	KeyStatus      bool   // --key-status
	ConvertSecret  string // --convert-secret
	FIsInline      bool   // -f 为内联清单（由 CheckArguments 判定）

	// -a 与 [interactive]：原始值在 Parse 里装好，解析结果由 CheckArguments 填——
	// 来源判定与门控只做一次，执行侧直接用。
	Answer  []string         // -a 的原始值（内联，或一个 CSV 文件路径），可重复
	Answers []ssh.Answer     // -a 解析后的代填表（顺序即命令行给出顺序）
	// AnswerFiles 是 -a 里文件来源的路径（顺序同命令行、不含内联值），归档备份用。
	AnswerFiles []string
	Match       ssh.MatchOptions // [interactive] 的匹配口径：管触发词与中止词

	// --sudo / --no-sudo 是否在命令行出现：互斥判定与「密钥管理命令不与批量参数同给」都要用
	sudoFlag, noSudoFlag bool

	// -t / -T / -n 的原始输入：非法值不在解析期报错，留给 CheckArguments
	// 按旧版口径报「参数格式错误」。
	timeoutRaw, connectTimeoutRaw, numberRaw             string
	timeoutInvalid, connectTimeoutInvalid, numberInvalid bool
}

// ModeName 返回本次运行的模式名（command / script / upload / download）。
// 四个值以 command > script > upload > download 的优先级判定；都没给时返回空串
// （正常流程走不到——参数合规检查已保证四者必有一个，空串只作防御）。
//
// 全工具单一判据点：归档目录名、报告、日志文案此前各自重判一遍同一组字段。
func (a *Args) ModeName() string {
	switch {
	case a.Command != "":
		return "command"
	case a.Script != "":
		return "script"
	case a.Upload != "":
		return "upload"
	case a.Download != "":
		return "download"
	}
	return ""
}

// NumberGiven 本次是否显式指定了 -n（没指定即"全部并行"）。
func (a *Args) NumberGiven() bool { return a.numberRaw != "" }

// Summary 把解析结果打印成旧版 argparse.Namespace 的样子（工具日志用）：
// 单行 `字段=值` 平铺，字段名跟着当前选项名走（c / s / u / d / f / p / sudo / t / T / n / r
// / k 与 no_bash / yes），未指定的字符串打印成 ”、未指定的数值打印成 None。
//
// 不复刻的只有两处：旧版把内联清单也塞进 f（靠 f_is_inline 二次判断），这里 f 只装
// 清单原文、内联与否由 FIsInline 单独报；旧版没有 --key-status 与 -a，故它俩排在后面。
func (a *Args) Summary() string {
	// 未指定：字符串与 '' 同形，数值与 None 同形（对位 argparse 的默认值）
	orEmpty := func(s string) string { return "'" + s + "'" }
	// 数值字段报「本次实际生效的值」：显式指定过、或程序补过默认值（-t/-T 的模式默认、
	// 配置里的 -m）都算生效；两者都没有才是 None。只看原始输入会把补出来的默认值漏掉。
	orNone := func(v int) string {
		if v == 0 {
			return "None"
		}
		return strconv.Itoa(v)
	}
	keyVal := boolPy(a.Key)

	fields := []string{
		"c=" + orEmpty(a.Command),
		"s=" + orEmpty(a.Script),
		"u=" + orEmpty(a.Upload),
		"d=" + orEmpty(a.Download),
		"f=" + orEmpty(common.MaskInlineListIf(a.CsvFile)),
		"p=" + orEmpty(a.Path),
		"sudo=" + boolPy(a.Sudo),
		"t=" + orNone(a.Timeout),
		"T=" + orNone(a.ConnectTimeout),
		"n=" + orNone(a.Number),
		"r=" + orEmpty(a.Remark),
		"no_bash=" + boolPy(a.NoBash),
		"yes=" + boolPy(a.Disinteractive),
		"k=" + keyVal,
		"answer=" + answerList(a.Answer),
	}
	if a.KeyStatus {
		fields = append(fields, "key_status=True")
	}
	return "Namespace(" + strings.Join(fields, ", ") + ")"
}

// answerList -a 的日志形态（值原文，逗号分隔的那一串）：给了就是 ['1,请选择架构']，没给就是 []。
//
// 取原始值而不是解析后的代填表：这行日志记的是「命令行写了什么」，而它打在参数合规检查
// 之前——那时代填表还没解析出来（展开后的明细在报告里，见 report.go）。
func answerList(values []string) string {
	items := make([]string, 0, len(values))
	for _, v := range values {
		items = append(items, "'"+v+"'")
	}
	return "[" + strings.Join(items, ", ") + "]"
}

// boolPy 把 Go 布尔打印成 Python 的 True / False（拼写不同，别混用）。
func boolPy(v bool) string {
	if v {
		return "True"
	}
	return "False"
}

// translateFlagError 把 pflag 的报错翻成中文。
// 这些全属"工具使用错误"（L49）：第一行就要让人看懂错在哪，而 pflag 的原文是
// Go 的行话（unknown flag / flag needs an argument），对英语不好的人是第二重门槛。
// 只翻"原因"那句，末尾的"使用 -h 查看帮助"由调用方统一追加。
// 命中的三类覆盖了本工具旗标集能产生的全部解析错误——字符串旗标什么值都收、
// 不会解析失败，取值非法只可能出在布尔旗标上；其余原样透传。
func translateFlagError(err error) error {
	var notExist *pflag.NotExistError
	if errors.As(err, &notExist) {
		if short := notExist.GetSpecifiedShortnames(); short != "" {
			return fmt.Errorf("不认识的选项：-%s", short)
		}
		return fmt.Errorf("不认识的选项：--%s", notExist.GetSpecifiedName())
	}

	var required *pflag.ValueRequiredError
	if errors.As(err, &required) {
		if short := required.GetSpecifiedShortnames(); short != "" {
			return fmt.Errorf("选项 -%s 缺少值", short)
		}
		return fmt.Errorf("选项 --%s 缺少值", required.GetSpecifiedName())
	}

	var invalid *pflag.InvalidValueError
	if errors.As(err, &invalid) {
		return fmt.Errorf("选项 --%s 的值只能是 true 或 false，当前值：%s",
			invalid.GetFlag().Name, invalid.GetValue())
	}
	return err
}

// Parse 解析命令行并补默认值。raw 是 os.Args[1:]，version 是入口定义的版本号（帮助显示用）。
func Parse(cfg *config.Config, version string, raw []string) (*Args, error) {
	fs := pflag.NewFlagSet("SSHFleet", pflag.ContinueOnError)
	fs.SetOutput(io.Discard) // 解析错误与提示全部自撰，不走 pflag 默认输出

	var a Args
	var sudoFlag, noSudoFlag bool
	fs.StringVarP(&a.Command, "command", "c", "", "远程在多台服务器上执行一条命令")
	fs.StringVarP(&a.Script, "script", "s", "", "远程在多台服务器上执行一个本地脚本")
	fs.StringVarP(&a.Upload, "upload", "u", "", "把本地文件或目录传到服务器")
	fs.StringVarP(&a.Download, "download", "d", "", "从服务器下载文件或目录到本地")
	fs.StringVarP(&a.CsvFile, "csv-file", "f", "", "节点清单：CSV 文件路径，或内联一行节点信息")
	fs.StringVarP(&a.Path, "path", "p", "", "目标路径：上传到服务器的目录 / 下载到的本地目录")
	fs.BoolVar(&sudoFlag, "sudo", false, "这次以 root 身份执行")
	fs.BoolVar(&noSudoFlag, "no-sudo", false, "这次以登录用户身份执行")
	fs.StringVarP(&a.timeoutRaw, "timeout", "t", "", "命令跑完、文件传完的最长等待（秒）")
	fs.StringVarP(&a.connectTimeoutRaw, "connect-timeout", "T", "", "连上服务器的最长等待（秒）")
	fs.StringVarP(&a.numberRaw, "number", "n", "", "并发数：同时操作几台服务器")
	fs.StringVarP(&a.Remark, "remark", "r", "", "备注，用作历史记录文件夹名（不填自动生成）")
	fs.BoolVar(&a.NoBash, "no-bash", false, "命令模式专用: 不套一层 bash 环境")
	fs.BoolVar(&a.Disinteractive, "yes", false, "跳过所有确认提示直接执行")
	// -a 必须用 StringArray，不能用 StringSlice：后者会先按逗号把值拆开，逗号就永远传不进来
	fs.StringArrayVarP(&a.Answer, "answer", "a", nil, "代填：看到触发词就自动填内容")
	fs.BoolVarP(&a.Key, "key", "k", false, "用密钥登录：私钥取清单第 5 列，其次配置 account.key")
	fs.BoolVar(&a.GenKey, "gen-key", false, "生成随机主密钥并持久化到 SSHFLEET_KEY")
	fs.BoolVar(&a.KeyStatus, "key-status", false, "查看主密钥状态：两处来源、指纹、是否一致与下一步")
	fs.StringVar(&a.ConvertSecret, "convert-secret", "", "转换凭据文件（跟目标文件路径）")

	// 未提供任何参数：打印帮助后以 0 退出（与旧版一致，发生在配置加载之后）
	if len(raw) == 0 {
		Usage(cfg, version)
		return nil, ErrHelp
	}

	if err := fs.Parse(raw); err != nil {
		if errors.Is(err, pflag.ErrHelp) {
			Usage(cfg, version)
			return nil, ErrHelp
		}
		return nil, fmt.Errorf("%v（使用 -h 查看帮助）", translateFlagError(err))
	}

	// 多出来的位置参数：一律报错。
	// 常见来源三类——① 通配符被终端展开（`-u /x/abc/*` 会变成多个路径，而 -u 只吃第一个，
	// 其余会落到这里）② 路径含空格未加引号 ③ 参数多打或打错。
	// 不拦的话它们被静默忽略，会造成"看起来成功、实际少传"（2026-09-14 实测）。
	if extra := fs.Args(); len(extra) > 0 {
		return nil, fmt.Errorf(
			"出现多余的参数（未被使用）：%s\n常见原因：\n"+
				"  ① 通配符被终端展开——`-u /x/abc/*` 会变成多个路径。上传目录不需要通配符，"+
				"直接写目录本身即可（目录内容会按原有层级传过去）\n"+
				"  ② 路径含空格但没加引号——请写成 \"路径 含 空格\"\n"+
				"  ③ 参数打多了或打错了",
			strings.Join(extra, " "))
	}

	// -t / -T / -n：转 int，非法留给 CheckArguments 报错
	a.Timeout, a.timeoutInvalid = toInt(a.timeoutRaw)
	a.ConnectTimeout, a.connectTimeoutInvalid = toInt(a.connectTimeoutRaw)
	a.Number, a.numberInvalid = toInt(a.numberRaw)

	// 未指定时的默认值（与旧版一致）
	switch {
	case a.Command != "" || a.Script != "":
		if a.timeoutRaw == "" {
			a.Timeout = cfg.Execution.TimeoutExecute
		}
	case a.Upload != "" || a.Download != "":
		if a.timeoutRaw == "" {
			a.Timeout = cfg.Execution.TimeoutTransfer
		}
	}
	if a.connectTimeoutRaw == "" {
		a.ConnectTimeout = cfg.Execution.TimeoutConnect
	}
	if a.numberRaw == "" {
		a.Number = 0
	}

	// 执行身份：两个开关互斥，都没给就用配置里的值（`--sudo`/`--no-sudo` 的冲突在 CheckArguments 报错）
	a.sudoFlag, a.noSudoFlag = sudoFlag, noSudoFlag
	switch {
	case sudoFlag:
		a.Sudo = true
	case noSudoFlag:
		a.Sudo = false
	default:
		a.Sudo = cfg.Execution.Sudo
	}

	// 匹配口径取自配置（[interactive]）：执行侧从 Args 取，不再回头读配置
	a.Match = ssh.MatchOptions{Regex: cfg.Interactive.Regex, CaseSensitive: cfg.Interactive.CaseSensitive}

	// 路径参数：中间不能含空格；再做字符串层规范化（顺序与旧版一致）
	for _, val := range []*string{&a.Script, &a.CsvFile, &a.Upload, &a.Path, &a.Download} {
		if *val == "" {
			continue
		}
		if strings.Contains(strings.TrimSpace(*val), " ") {
			return nil, fmt.Errorf("路径参数中间不能包含空格\n提示：路径里有空格时用引号包起来，例如 -u \"D:/my dir/app\"")
		}
		*val = normalizePath(*val)
	}

	a.Remark = defaultRemark(&a)
	return &a, nil
}

func toInt(raw string) (int, bool) {
	if raw == "" {
		return 0, false
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return 0, true
	}
	return v, false
}

// normalizePath 字符串层面的路径规范化（对位旧 args_normalize_path）：
// 统一分隔符为 /、处理 . 与 ..、不增删结尾 /、Windows 盘符转大写 C:/ 形式。
func normalizePath(p string) string {
	p = strings.TrimSpace(p)
	p = strings.ReplaceAll(p, "\\", "/")
	endsSlash := strings.HasSuffix(p, "/")

	if len(p) >= 2 && p[1] == ':' && isDriveLetter(p[0]) {
		rest := strings.TrimLeft(p[2:], "/")
		if rest == "" {
			return strings.ToUpper(p[:1]) + ":/"
		}
		return strings.ToUpper(p[:1]) + ":/" + rest
	}

	cleaned := path.Clean(p)
	if endsSlash && cleaned != "/" {
		cleaned += "/"
	}
	return cleaned
}

func isDriveLetter(b byte) bool { return (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z') }

// defaultRemark 未指定 -r 时自动生成：命令取首词前 8 字符（特殊字符换下划线），
// 脚本 / 上传 / 下载取路径文件名。
func defaultRemark(a *Args) string {
	if a.Remark != "" {
		return a.Remark
	}
	switch {
	case a.Command != "":
		fields := strings.Fields(a.Command)
		if len(fields) == 0 {
			return ""
		}
		first := []rune(fields[0])
		if len(first) > 8 {
			first = first[:8]
		}
		return strings.NewReplacer(
			" ", "_", "/", "_", ":", "_", "*", "_", "?", "_",
			"\"", "_", "<", "_", ">", "_", "|", "_", "\\", "_",
		).Replace(string(first))
	case a.Script != "":
		return path.Base(a.Script)
	case a.Upload != "":
		return path.Base(a.Upload)
	case a.Download != "":
		return path.Base(a.Download)
	}
	return ""
}

// Usage 打印帮助（未提供任何参数或 -h 时）：首行下方显示版本号（含构建标识）。
func Usage(cfg *config.Config, version string) {
	fmt.Print(usageText(cfg, version, termWidth()))
}

// termWidth 当前终端宽度；非终端（管道/重定向）或取不到时返回 0（表示不折行）。
func termWidth() int {
	w, _, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil || w <= 0 {
		return 0
	}
	return w
}

// helpEntry 帮助选项表的一行：四列各占一列、互不串行——
// 短选项 / 长选项 / 方括号标记 / 详细说明（超宽时在本列内折行）。
type helpEntry struct {
	short string // 无短选项时为空
	long  string
	tag   string // [默认: 10] / [当前配置: 开] 这类标记，无则空
	desc  string
	blank bool   // 分组用的空行
	group string // 组标题行（只有这一项非空）
}

// blankRow 选项表的分组空行。
var blankRow = helpEntry{blank: true}

// groupRow 选项表的组标题行（纯文字加缩进，不用横线装饰）。
func groupRow(title string) helpEntry { return helpEntry{group: title} }

// opt 构造一行选项（blank / group 恒为空，不必逐个写字面量字段）。
func opt(short, long, tag, desc string) helpEntry {
	return helpEntry{short: short, long: long, tag: tag, desc: desc}
}

// helpEntries 选项表内容：描述力求简洁、清晰、明确——能省的字省掉，
// 「必填 / 取值语义 / 默认值」这些影响使用的信息一个不省。
// 五组：模式（四选一）/ 清单与目标路径 / 执行参数 / 密钥与凭据 / 密钥管理，组间空行分隔。
func helpEntries(cfg *config.Config) []helpEntry {
	sudoTag, noSudoDesc := "[当前配置: 关]", "这次以登录用户身份执行"
	if cfg.Execution.Sudo {
		sudoTag, noSudoDesc = "[当前配置: 开]", "这次以登录用户身份执行（配置是开时用它改回来）"
	}
	return []helpEntry{
		groupRow("模式（四选一）"),
		opt("-c", "--command", "", "在多台服务器上执行一条命令"),
		opt("-s", "--script", "", "在多台服务器上执行一个本地脚本"),
		opt("-u", "--upload", "", "把本地文件或目录上传到服务器"),
		opt("-d", "--download", "", "从服务器下载文件或目录到本地"),

		blankRow,

		groupRow("清单与目标路径"),
		opt("-f", "--csv-file", "", "节点清单：CSV 文件路径，或直接写一行节点信息（必填）"),
		opt("-p", "--path", "", "目标路径：上传写服务器目录 / 下载写本机目录（上传、下载必填）"),

		blankRow,

		groupRow("执行参数"),
		opt("-n", "--number", "[默认: 全部]", "并发数：同时操作几台，不填=全部并行"),
		opt("-r", "--remark", "", "备注，用作历史记录文件夹名（不填自动生成）"),
		opt("", "--sudo", sudoTag, "这次以 root 身份执行"),
		opt("", "--no-sudo", "", noSudoDesc),
		opt("", "--yes", "", "跳过所有确认直接执行（自动化用，用它之前先手动跑通一次）"),
		opt("-a", "--answer", "", "代填：看到触发词就自动填内容（值形如「代填内容,触发词,触发词」），也可给 CSV 文件的路径（每行一条）"),
		opt("", "--no-bash", "", "命令模式：不套 bash，直接执行原始命令"),
		opt("-t", "--timeout", fmt.Sprintf("[默认: %d/%d]", cfg.Execution.TimeoutExecute, cfg.Execution.TimeoutTransfer), "命令跑完、文件传完的最长等待（秒）"),
		opt("-T", "--connect-timeout", fmt.Sprintf("[默认: %d]", cfg.Execution.TimeoutConnect), "连上服务器的最长等待（秒）"),

		blankRow,

		groupRow("密钥与凭据"),
		opt("-k", "--key", "", "密钥登录：不写=只用密码；写了=用清单或配置里的私钥"),

		blankRow,

		groupRow("密钥管理（不执行批量任务）"),
		opt("", "--gen-key", "", "生成主密钥（打开加密之后才会用到），密钥存进环境变量 SSHFLEET_KEY"),
		opt("", "--key-status", "", "查看主密钥状态：读到哪把、已存哪把、是否一致"),
		opt("", "--convert-secret", "", "转换凭据文件（后面跟要转的文件路径）：按配置里的加密开关，转成明文或密文"),
	}
}

// 帮助表的列间留白：短选项与长选项之间只留 1 个空格（贴成一个整体），
// 其余列之间留 2 个空格。测试按同一组常量核对列位置，避免两边各写一份。
const (
	helpGroupIndent = "  "   // 组标题缩进
	helpIndent      = "    " // 选项行缩进（比组标题深一级，一眼看出谁属于谁）
	helpOptGap      = " "
	helpGap         = "  "
)

// minDescWidth 说明列保底宽度：终端太窄时宁可整行超宽，也不把说明挤成一列一个字。
const minDescWidth = 24

// usageText 渲染帮助文本。width 为终端宽度，<=0 表示不折行（管道下长行更便于 grep）。
// 抽成纯函数：列对齐与折行都要能被断言。
func usageText(cfg *config.Config, version string, width int) string {
	name := filepath.Base(os.Args[0])
	entries := helpEntries(cfg)

	wShort, wLong, wTag := 0, 0, 0
	for _, e := range entries {
		if e.blank || e.group != "" {
			continue
		}
		wShort = max(wShort, common.DisplayWidth(e.short))
		wLong = max(wLong, common.DisplayWidth(e.long))
		wTag = max(wTag, common.DisplayWidth(e.tag))
	}
	padTo := func(s string, w int) string {
		if d := common.DisplayWidth(s); d < w {
			return s + strings.Repeat(" ", w-d)
		}
		return s
	}
	// 说明列的起始列与可用宽度：折行必须落在说明列内，不串到其它列
	descStart := common.DisplayWidth(helpIndent) + wShort + common.DisplayWidth(helpOptGap) +
		wLong + common.DisplayWidth(helpGap) + wTag + common.DisplayWidth(helpGap)
	descWidth := 0
	if width > 0 {
		descWidth = max(width-descStart-1, minDescWidth)
	}

	var b strings.Builder
	b.WriteString("SSHFleet - 批量 SSH 运维工具（命令/脚本执行、文件上传下载）\n")
	b.WriteString(fmt.Sprintf("版本: v%s\n\n", version))
	b.WriteString("用法:\n")
	b.WriteString(helpGroupIndent + fmt.Sprintf("%s -c | -s | -u | -d 之一，配合 -f 批量执行\n", name))
	b.WriteString(helpGroupIndent + fmt.Sprintf("%s --gen-key | --key-status | --convert-secret 密钥管理\n\n", name))
	b.WriteString("选项:\n\n")
	for _, e := range entries {
		switch {
		case e.group != "": // 组标题
			b.WriteString(helpGroupIndent + e.group + "\n")
			continue
		case e.blank: // 分组空行
			b.WriteString("\n")
			continue
		}
		head := helpIndent + padTo(e.short, wShort) + helpOptGap + padTo(e.long, wLong) +
			helpGap + padTo(e.tag, wTag) + helpGap
		for i, line := range common.WrapByWidth(e.desc, descWidth) {
			if i == 0 {
				b.WriteString(head + line + "\n")
				continue
			}
			b.WriteString(strings.Repeat(" ", descStart) + line + "\n")
		}
	}
	b.WriteString("\n示例:\n")
	examples := [][2]string{
		{"执行命令:", fmt.Sprintf("%s -f nodes.csv -c \"ls -l\"", name)},
		{"执行脚本:", fmt.Sprintf("%s -f nodes.csv -s deploy.sh", name)},
		{"代填执行:", fmt.Sprintf("%s -f nodes.csv -s deploy.sh -a \"1,请选择架构\"", name)},
		{"上传文件:", fmt.Sprintf("%s -f nodes.csv -u ./dist/app -p /opt/app/", name)},
		{"下载文件:", fmt.Sprintf("%s -f nodes.csv -d /var/log/app -p ./logs/", name)},
		{"用密钥登录:", fmt.Sprintf("%s -f nodes.csv -c \"uptime\" -k", name)},
		{"不确认执行:", fmt.Sprintf("%s -f nodes.csv -c \"uptime\" --yes", name)},
	}
	labelWidth := 0
	for _, e := range examples {
		labelWidth = max(labelWidth, common.DisplayWidth(e[0]))
	}
	for _, e := range examples {
		b.WriteString("  " + padTo(e[0], labelWidth) + " " + e[1] + "\n")
	}
	b.WriteString("\n更多用法、配置说明、示例：见 docs/\n")
	return b.String()
}
