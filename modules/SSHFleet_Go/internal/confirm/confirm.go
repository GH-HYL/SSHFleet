// Package confirm 承载主干第 7 步：参数信息交互确认（M2 落地）。
package confirm

import (
	"fmt"

	"sshfleet/internal/cli"
	"sshfleet/internal/config"
	"sshfleet/internal/nodelist"
)

// errNotImplemented 占位：M2 落地后删除。
var errNotImplemented = fmt.Errorf("未实现：M2 数据层落地")

// Confirm 展示参数与节点信息供用户核实；非交互模式显式返回「确认」，
// 不再借用 yorn 参数值（spec 实现层差异简记）。
func Confirm(a *cli.Args, nodes *nodelist.Nodes, cfg *config.Config) error {
	_, _, _ = a, nodes, cfg
	return errNotImplemented
}
