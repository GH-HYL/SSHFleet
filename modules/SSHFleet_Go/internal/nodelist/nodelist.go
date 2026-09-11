// Package nodelist 承载主干第 6 步：读取节点清单 + 字段补全 + 输入记忆（M2 落地）。
//
// 节点标识只支持 IPv4 字面量，严格校验（spec D13）；表头识别规则见 spec
// 实现层差异简记；CSV 读取必须 FieldsPerRecord = -1（容忍变长行）。
package nodelist

import (
	"fmt"

	"sshfleet/internal/cli"
	"sshfleet/internal/config"
)

// errNotImplemented 占位：M2 落地后删除。
var errNotImplemented = fmt.Errorf("未实现：M2 数据层落地")

// Nodes 节点信息集合（纯数据载体，M2 定稿字段）。
type Nodes struct{}

// Read 读取节点清单（CSV 文件或 -f 内联），完成字段补全与输入记忆。
func Read(a *cli.Args, cfg *config.Config) (*Nodes, error) {
	_, _ = a, cfg
	return nil, errNotImplemented
}
