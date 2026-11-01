package contract

// Plan 是「解析交闸门」那张纸（口径：这次要做什么）。
//
// 预填充：本阶段只装四格——这次干什么、执行参数、判据表、解析时发现的问题。
// 还缺三格（对谁做／执行资源／运行上下文），随阶段 04 补齐；加格子是加字段，不是推倒重来。
//
// 判据见 ADR-0035 决定一；「闸门只加结论、不加数据」见决定三。
type Plan struct {
	Mode     Mode
	Task     Task
	Run      RunParams
	Tables   Tables
	Problems []Problem
}

// RunParams 执行参数：并发、三个超时、执行身份，加几个本阶段就要用的开关。
type RunParams struct {
	Concurrency     int // 0 表示不填，按全部并行
	TimeoutConnect  int
	TimeoutExecute  int
	TimeoutTransfer int
	Sudo            bool
	NoShell         bool
	Yes             bool // 非交互：不问任何问题，直接按缺省放行
	Key             bool // 用密钥登录
	Remark          string
}

// ProblemKind 是「解析时发现的问题」的种类。解析只发现、不下结论。
type ProblemKind string

const (
	ProblemUnknownOption ProblemKind = "unknown_option" // 表里没有这个选项
	ProblemMissingValue  ProblemKind = "missing_value"  // 带值选项后面没有值
	ProblemBadValue      ProblemKind = "bad_value"      // 值认不出（例如 -n abc）
	ProblemMultiMode     ProblemKind = "multi_mode"     // 同时给了多个模式
)

// Problem 一条事实：哪个选项、原值是什么、哪一处对不上。
//
// 它不带结论，也不带句子——判「这条不过」是闸门的活，拼话是 message 的活。
type Problem struct {
	Kind   ProblemKind
	Option string // 选项名，例如 -n
	Raw    string // 命令行上给的原值，没给值时为空
	Detail string // 一句事实说明，例如「必须是正整数」
}
