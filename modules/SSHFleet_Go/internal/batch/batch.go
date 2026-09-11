// Package batch 承载主干第 8 步：并发执行 SSH / SFTP + 进度聚合（M3 落地）。
//
// 进度聚合器住这里（跨调用保存状态，与 worker pool 同一生命周期，spec D2）；
// main 把 internal/output 的渲染函数作为参数注入——这是依赖注入，不是
// 回调控制生命周期（渲染函数只负责画，不决定流程起止）。
package batch

import (
	"fmt"

	"sshfleet/internal/cli"
	"sshfleet/internal/config"
	"sshfleet/internal/log"
	"sshfleet/internal/nodelist"
)

// errNotImplemented 占位：M3 落地后删除。
var errNotImplemented = fmt.Errorf("未实现：M3 执行层落地")

// Results 执行结果集合（纯数据载体，M3 定稿字段）。
type Results struct{}

// Run 并发执行四种模式并聚合进度。
func Run(a *cli.Args, cfg *config.Config, nodes *nodelist.Nodes, logger *log.Logger) (*Results, error) {
	_, _, _, _ = a, cfg, nodes, logger
	return nil, errNotImplemented
}
