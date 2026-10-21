package common

import (
	"net/netip"
	"strings"
)

// IsIPv4Literal 是不是一个合法的 IPv4 字面量（D13 的节点标识口径）。
//
// 只认 IPv4：IPv6、带端口、带掩码、主机名一律不算；首尾空白先去掉。
// 四处（节点清单字段校验 / 归档清单表头识别 / -f 内联文本判定 / 内联清单的脱敏）
// 都走这一处，`"10.0.0.1"` 这类 CSV 引号形态由调用方先按 CSV 读成字段再进来。
func IsIPv4Literal(s string) bool {
	addr, err := netip.ParseAddr(strings.TrimSpace(s))
	return err == nil && addr.Is4()
}
