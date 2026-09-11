// Package dangercheck 承载主干第 5 步的危险命令检测（M4 落地）。
//
// 解析层用 mvdan.cc/sh/v3/syntax（spec D25），判定层（平级正则 + 旗标归一化 +
// 等级排序）自己实现；多命中全列、风险降序（D34）；非交互非 forbidden 放行留痕（D35）。
// 规则文件 TOML，语义与正则一字不动（D11）。
package dangercheck

import (
	"fmt"

	"sshfleet/internal/cli"
)

// errNotImplemented 占位：M4 落地后删除。
var errNotImplemented = fmt.Errorf("未实现：M4 判定层落地")

// Check 对命令内容做静态分类（命令段 + 命中规则 + 风险等级）。
// 定位是操作助手，不是拦截恶意命令的防御闸门（CONTEXT.md「危险检测」）。
func Check(a *cli.Args) error {
	_ = a
	return errNotImplemented
}
