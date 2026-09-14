//go:build windows

package credential

import (
	"fmt"
	"os/exec"
	"strings"

	"golang.org/x/sys/windows/registry"
)

// Windows 侧主密钥持久化：读取走注册表直读（实现途径 7，不起 reg 子进程），
// 写入维持 setx（与旧实现一致——写注册表 Environment 键需广播 WM_SETTINGCHANGE 才能让
// 新开的进程看到，setx 替我们做了这件事）。

// readPersistedKey 从 HKCU\Environment 读取已存主密钥，无则返回空串。
func readPersistedKey() string {
	k, err := registry.OpenKey(registry.CURRENT_USER, `Environment`, registry.QUERY_VALUE)
	if err != nil {
		return ""
	}
	defer k.Close()
	val, _, err := k.GetStringValue(envName)
	if err != nil || strings.TrimSpace(val) == "" {
		return ""
	}
	return val
}

// persistKey 用 setx 写入注册表（当前终端读不到新值，需重开终端）。
func persistKey(key string, regenerated bool, out *strings.Builder) error {
	actionDesc := "生成"
	if regenerated {
		actionDesc = "重新生成"
	}
	cmd := exec.Command("setx", envName, key)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("主密钥自动保存失败：%v\n可手动保存：执行 setx %s 你的随机密钥", err, envName)
	}
	fmt.Fprintf(out, "主密钥已%s，并自动保存到本机\n", actionDesc)
	fmt.Fprintln(out, "请重新打开终端后再使用（当前终端读不到新密钥）")
	return nil
}
