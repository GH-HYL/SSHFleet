package contract

// 中间步骤：工具在远端额外跑的那些伴随命令，各自一条退出码。
//
// 它们的退出码**不进 `NodeResult.ExitCode`**：那一格装的是「目的命令」的退出码，
// 中间步骤没跑成与目的命令失败是两件事（老版 result-verdict spec D30／D33 的口径）。

// 中间步骤的语义名（`Step.Name` 的取值）。来源冻结在三类工具内部的伴随命令：
// 以后执行路径里出现新的中间命令，必须连这份清单一起补议，不许顺手 append。
// 裸命令原文（`test -e`、`find`、`:`）不在这里——那是呈现层的话，归 message。
const (
	StepPrecheck  = "precheck"  // 下载预检查，命令 test -e
	StepEnumerate = "enumerate" // 远程文件枚举，命令 find
	StepPasswd    = "passwd"    // 改密收尾的空命令，命令 :
)

// Step 一条中间步骤的退出码。拿不到（超时、中断）时 ExitCode 为 nil，不推 0。
type Step struct {
	Name     string
	ExitCode *int
}
