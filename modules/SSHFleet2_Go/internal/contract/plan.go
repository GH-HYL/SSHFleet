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
	// 由塔尖在往下递之前打出去——顺序与老版一致：提示在前、确认屏在后。
	Notices []Text
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
