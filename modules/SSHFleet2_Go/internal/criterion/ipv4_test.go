package criterion

import "testing"

// 是不是合法 IPv4：前导零、超界、段数不足一律不认，前后空白算书写习惯。

func TestIsIPv4(t *testing.T) {
	for _, bad := range []string{
		"010.0.0.1", "10.00.0.1", "10.0.0.01", // 前导零不是合法写法（审计 06-H1）
		"10.0.0", "10.0.0.256", "10.0.0.1.5", // 段数不对、超界
		"", "  ", "::1", "10.0.0.1/24", // 空、IPv6、带掩码
	} {
		if IsIPv4(bad) {
			t.Errorf("%q 不该认成合法 IPv4", bad)
		}
	}
	for _, ok := range []string{
		"10.0.0.1", "0.0.0.0", "127.0.0.1", "255.255.255.255",
		" 10.0.0.1 ", "\t10.0.0.1\n", // 前后空白不影响判断
	} {
		if !IsIPv4(ok) {
			t.Errorf("%q 应当认成合法 IPv4", ok)
		}
	}
}
