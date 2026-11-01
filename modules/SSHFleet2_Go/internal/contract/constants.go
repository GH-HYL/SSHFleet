package contract

// 四档风险等级的取值。**顺序不在这里**——契约只许类型与常量，而「顺序」在 Go 里落成
// 一张 map 就是变量；顺序只住闸门一处（它排命中先后），解析只校验「在不在四档里」。
const (
	RiskForbidden = "forbidden"
	RiskHigh      = "high"
	RiskMedium    = "medium"
	RiskLow       = "low"
)

// 工具日志的落点：目录名与文件名都是内置取值，不开放配置。
//
// 预填充：ADR-0032 说「路径由 archive 给」，而归档包要到阶段 04 才有；在那之前由塔尖把
// 这两格递给日志包，日志包自己不记路径。归档包落地后改由它发，日志包入口签名不变。
const (
	HistoryDir  = "history"
	ToolLogName = "SSHFleetTools.log"
)
