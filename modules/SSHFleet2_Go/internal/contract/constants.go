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
