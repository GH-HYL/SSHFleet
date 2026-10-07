package common

import (
	"net/netip"
	"strings"
)

// IsIPv4Literal 是不是一个合法的 IPv4 字面量（D13 的节点标识口径）。
//
// 只认 IPv4：IPv6、带端口、带掩码、主机名一律不算；首尾空白先去掉。
// 三处（节点清单字段校验 / 归档清单表头识别 / -f 内联文本判定）曾各持一份逐字相同的
// 实现，判定口径只此一处。
//
// 注意与 mask.go 的 looksLikeIPv4 不是一回事：那个只判形状、不校验取值范围，
// 用途是决定要不要脱敏，不是判节点合法性。
func IsIPv4Literal(s string) bool {
	addr, err := netip.ParseAddr(strings.TrimSpace(s))
	return err == nil && addr.Is4()
}
