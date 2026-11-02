package contract

// 四档风险等级的取值。**顺序不在这里**——契约只许类型与常量，而「顺序」在 Go 里落成
// 一张 map 就是变量；顺序只住闸门一处（它排命中先后），解析只校验「在不在四档里」。
const (
	RiskForbidden = "forbidden"
	RiskHigh      = "high"
	RiskMedium    = "medium"
	RiskLow       = "low"
)

// HistoryDir 是历史记录的根目录名：内置取值，不开放配置。
//
// 它与本次归档目录里那几个文件名不同族——那几个住在 `archive`（谁拼路径谁管名字），
// 而这一格是塔尖要递给归档包的**根**，所以留在这里：`archive` 按权限表不依赖契约，
// 根只能由调用方给（03 号单的落地口径）。
const HistoryDir = "history"

// WhereOutput 是「起不来」发生在哪一环的取值之一：写归档目录那一步。
//
// 其余几个取值（cli／nodelist／config／verdict／dangercheck／ssh）各只在一个站里出现，
// 住在那个站里就够；这一个跨两个包——执行站建终端输出文件失败时用它，塔尖建归档目录、
// 写 `report.txt` 失败时也用它，所以提到契约：改一次两处一起变。
const WhereOutput = "output"

// FlagPairSudo 是「两个身份开关同时给」这条事实的标签。
//
// 解析站把它记进 `Problem.Option`，闸门按它认出是哪一对冲突——两处比的是同一个串，
// 所以只许有这一份：一处改了另一处没改，闸门那条判据会静默不响。
const FlagPairSudo = "--sudo --no-sudo"
