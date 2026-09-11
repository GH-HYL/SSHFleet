// Package result 承载主干第 9 步：结果统计 + 错误分类（M4 落地）。
//
// 错误分类语料出口见 modules/SSHFleet_bak/test/（test_error_classification.py
// + 4 份 CSV 样本）。
package result

import (
	"fmt"

	"sshfleet/internal/batch"
	"sshfleet/internal/cli"
	"sshfleet/internal/nodelist"
)

// errNotImplemented 占位：M4 落地后删除。
var errNotImplemented = fmt.Errorf("未实现：M4 判定层落地")

// Stats 统计结果（纯数据载体，M4 定稿字段）。
type Stats struct{}

// Statistics 计算统计结果并做错误分类。
func Statistics(results *batch.Results, nodes *nodelist.Nodes, a *cli.Args) (*Stats, error) {
	_, _, _ = results, nodes, a
	return nil, errNotImplemented
}
