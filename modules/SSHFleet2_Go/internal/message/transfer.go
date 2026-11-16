package message

// 上传下载的输出明细行——每台机器那一段里逐条列出的事。
//
// 这些行既上屏也进 `terminal-output.txt`，是使用者直接读的字，所以成形处也归这里。
// 原先写死在 `execute` 的 `run_upload.go` 与 `run_download.go` 里，两处各一份、字面量相同：
// 「已跳过（符号链接）」那句一字不差地写了两遍。

import (
	"fmt"

	"sshfleet2/internal/contract"
)

// TransferSkipped 被跳过的符号链接那一行：「<名字>: 已跳过（符号链接）」。
func TransferSkipped(name string) string {
	return name + ": 已跳过（符号链接）"
}

// TransferFailure 失败那一行：「<名字>: 上传失败 - <原因>」（下载同形，动词随模式）。
//
// reason 收 `any`：调用点有的给字符串、有的直接给 error，`%v` 对两者同形。
func TransferFailure(m contract.Mode, name string, reason any) string {
	return fmt.Sprintf("%s: %s失败 - %v", name, ActionName(m), reason)
}
