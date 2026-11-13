package message

import (
	"testing"

	"sshfleet2/internal/contract"
)

// 执行身份：要了 root 且没被 --no-shell 削掉才算 root，其余都算登录用户。
// 确认屏那一行与执行期日志那一格共用这一条，所以只在被测包测一次。

func TestIdentityLabel(t *testing.T) {
	for _, c := range []struct {
		sudo, noShell bool
		want          string
	}{
		{true, false, "root"},
		{true, true, "登录用户"},   // --no-shell 削掉了 sudo
		{false, false, "登录用户"}, // 没要 root
		{false, true, "登录用户"},
	} {
		got := IdentityLabel(contract.RunParams{Sudo: c.sudo, NoShell: c.noShell})
		if got != c.want {
			t.Errorf("sudo=%v noShell=%v：得到 %q，想要 %q", c.sudo, c.noShell, got, c.want)
		}
	}
}
