package contract

// Plan 是「解析交闸门」那张纸（口径：这次要做什么）。
//
// 七格齐了（建站 04 补齐）：这次干什么、对谁做、执行参数、判据表、执行资源、运行上下文、
// 解析时发现的问题。判据见 ADR-0035 决定一；「闸门只加结论、不加数据」见决定三。
type Plan struct {
	Mode   Mode
	Task   Task
	Nodes  []Node
	Run    RunParams
	Tables Tables

	// Resources 是这次用到的原料在哪：清单、脚本、代填的路径。
	Resources Resources

	// RunMeta 是运行上下文的密文：使用者敲的原值封在里面，只有报告站解得开。
	// 中间站（闸门、执行、结算）收下原样传，不该也读不到里面是什么。
	RunMeta RunMeta

	// Problems 是解析时发现的问题：只列事实，不下结论。
	Problems []Problem

	// Notices 是解析站路上要说给使用者听、但**不必中断**的那几句（例如「第一行不是 IP，
	// 已当表头移除」）。解析站自己不写终端（ADR-0032 第四条），所以把它们挂在纸上，
	// 由塔尖在往下递之前向 message 要那一句、打出去——顺序与老版一致：提示在前、确认屏在后。
	//
	// 装的是种类与事实，不是成品的话：话只能由 message 造（`message.Line`）。
	Notices []Notice
}

// RunParams 执行参数：并发、三个超时、执行身份，加几个本阶段就要用的开关。
type RunParams struct {
	Concurrency     int // 0 表示不填，按全部并行
	TimeoutConnect  int
	TimeoutExecute  int
	TimeoutTransfer int
	// Sudo 只有一问：这一次到底用不用。命令行上给了 `--sudo` 或 `--no-sudo` 就按给的算，
	// 都没给就留着配置里的值。
	//
	// 「两个都给」不在这里表示——那条事实由解析站记成 Problem（ProblemFlagConflict），
	// 闸门照它拒。老版另存两个旗标只为报这一句，代价是纸上多两格、每种读法都得自己
	// 推出结论；现在纸上只有结论，事实走事实那条路。
	Sudo    bool
	NoShell bool
	Yes     bool   // 非交互：不问任何问题，直接按缺省放行
	Key     bool   // 用密钥登录
	Answer  string // 代填的原值：改密与它互斥，04／05 还要拿它做材料检查
	Remark  string

	// SudoGiven 是「命令行上到底给没给身份开关」（`--sudo` 或 `--no-sudo`）。
	//
	// Sudo 那一格只有结论（这次用不用），而改密那两条互斥问的是「给没给」——配置默认是
	// root 时，没给开关也不该报错。两条事实，两格。
	SudoGiven bool

	// MatchRegex 与 MatchCaseSensitive 是配置 `[interactive]` 那两格的原值：
	// 代填与改密认触发词时按不按正则、区不区分大小写。执行站不读配置，只能从纸上来。
	MatchRegex         bool
	MatchCaseSensitive bool

	// ConcurrencyAdvice 是按上传总大小与配置阈值算出来的建议并发；0 表示没有建议。
	// 闸门拿它问那一句（问的时候可能把 Concurrency 改掉，见 ADR-0040）。
	ConcurrencyAdvice int

	// FallbackExt 是脚本后缀没配到解释器时那个后缀；空串表示配到了。
	// 闸门拿它问那句「继续用 bash 吗」——解析站只记事实，问与不问归闸门。
	FallbackExt string

	// Interpreter 是命令交给谁解释：配置 `interpreter.command` 的原值（例如 bash），
	// 命令模式下恒有、脚本模式按后缀另取（随阶段 05）。
	//
	// 它是**原值**，不是拼好的下发行：拼法全在执行站一处（ADR-0015），纸只负责把配置
	// 里的东西原样带到执行站——执行侧不回头读配置（ADR-0014、ADR-0015 的更新记录）。
	Interpreter string

	// Env 是要导入目标机的环境变量：配置 `[env]` 原样（名字 → 值），键名已由解析站校验过。
	//
	// 与 Interpreter 同一条理由：纸上是事实，`export …;` 那段怎么拼归执行站。
	// 出厂配置带 PATH 与 TERM=dumb——少了它们，sudo 与解释器会 command not found，
	// 远端程序还会上色出转义序列，与发布版的行为对不上。
	Env map[string]string

	// English 是配置 `[locale] english` 的原值：真则下发行里多一句 `export LC_MESSAGES=C`，
	// 目标机上**命令自己**的提示语统一说英文。
	//
	// 它只负责下发行那一句。判据表按语言选哪一套、提示语说什么，是阶段 08 的事——
	// 那一侧还没接，这一格先照配置原样带到执行站，免得下发行与发布版差一句。
	English bool

	// Downlink 是**成品**：这次要发给远端的那一条命令行，解析站拼好放上来。
	//
	// 拼装只此一处（ADR-0010 的补充、建站 05 规格第三节）：执行站只发不拼，量长度的那条串
	// 与发出去的串因此是同一个值。四条支路（命令、--no-shell、脚本、`-a`）都在解析站成形。
	Downlink string

	// Stdin 是普通路径要沿标准输入送下去的正文；`-a` 时为空（正文已 base64 编进命令行）。
	//
	// 正文仍留在纸上：命令与脚本两种模式从它走 stdin，命令行里只有固定形态的外壳。
	Stdin string
}

// ProblemKind 是「解析时发现的问题」的种类。解析只发现、不下结论。
type ProblemKind string

const (
	ProblemUnknownOption ProblemKind = "unknown_option" // 表里没有这个选项
	ProblemMissingValue  ProblemKind = "missing_value"  // 带值选项后面没有值
	ProblemBadValue      ProblemKind = "bad_value"      // 值认不出（例如 -n abc）
	ProblemMultiMode     ProblemKind = "multi_mode"     // 同时给了多个模式
	ProblemFlagConflict  ProblemKind = "flag_conflict"  // 一对互斥开关同时给了
	ProblemExtraArgs     ProblemKind = "extra_args"     // 多出几段位置参数：值里的空格没加引号

	// ProblemInterpreterUnsupported 是「`-a` 碰上表外的解释器」：解析站拼不出下发行，
	// 只记这条事实交给闸门。判「不过」仍只归闸门，解析站自己不下结论。
	ProblemInterpreterUnsupported ProblemKind = "interpreter_unsupported"
)

// Problem 一条事实：哪个选项、原值是什么、哪一处对不上。
//
// 它不带结论，也不带句子——判「这条不过」是闸门的活，拼话是 message 的活。
type Problem struct {
	Kind   ProblemKind
	Option string // 选项名，例如 -n；冲突那一类装的是两个开关名，空格分开；多余参数装的是它跟在谁后面
	Raw    string // 命令行上给的原值，没给值时为空；多余参数装的是那几段按原样拼起来
	Detail string // 一句事实说明，例如「必须是正整数」
}
