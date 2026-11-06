//go:build windows

package credential

import (
	"os/exec"
	"strings"

	"golang.org/x/sys/windows/registry"

	"sshfleet2/internal/contract"
)

// Windows 侧主密钥持久化：读走注册表直读（不起 reg 子进程），写走 `setx`。
//
// 写注册表 Environment 键要广播 WM_SETTINGCHANGE 才能让新开的进程看到，`setx` 替我们做了
// 这件事——所以不自己调 API。代价是**当前终端**仍是旧密钥，要重开终端才生效；这条事实
// 由预检查与 `--gen-key` 的收尾那句说给使用者。

// hostPlatform 说持久值落在哪一侧。
const hostPlatform = contract.KeyHostWindows

// registryWhere 是持久值的位置：提示里要原样写出来。
const registryWhere = `注册表 HKCU\Environment`

// rcFilePath Windows 没有 rc 文件。
func rcFilePath() string { return "" }

// readSourcesPlatform 读注册表里已保存的主密钥。
func readSourcesPlatform() Sources {
	return Sources{
		Persisted:      readPersistedKey(),
		PersistedWhere: registryWhere,
	}
}

// readPersistedKey 从 HKCU\Environment 读主密钥，没有就交回空串。
func readPersistedKey() string {
	k, err := registry.OpenKey(registry.CURRENT_USER, `Environment`, registry.QUERY_VALUE)
	if err != nil {
		return ""
	}
	defer func() { _ = k.Close() }()
	val, _, err := k.GetStringValue(EnvName)
	if err != nil || strings.TrimSpace(val) == "" {
		return ""
	}
	return strings.TrimSpace(val)
}

// persistKey 用 setx 把主密钥写进用户环境变量。
func persistKey(key string) (contract.PersistResult, error) {
	cmd := exec.Command("setx", EnvName, key)
	if err := cmd.Run(); err != nil {
		return contract.PersistResult{Where: registryWhere}, err
	}
	return contract.PersistResult{Where: registryWhere}, nil
}
