package criterion

import (
	"net/netip"
	"strings"
)

// IsIPv4 严格认 IPv4 字面量：四段、每段 0-255、不认前导零。
//
// 判据交给标准库（`netip.ParseAddr` ＋ `Is4`），与老版同一条。前后空白算书写习惯，不参与
// 判断——两层调用点拿到手的已经是切好的列，这里再剥一次是给内联记录那条路用的。
func IsIPv4(s string) bool {
	addr, err := netip.ParseAddr(strings.TrimSpace(s))
	return err == nil && addr.Is4()
}
