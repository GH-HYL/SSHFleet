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
//
// 由此产生一个 Windows 特有的坑：setx 之后**当前终端**的环境变量仍是旧密钥，
// 重开终端才会读到新的。故读密钥时做来源一致性检测（见 masterkey.go），
// 加密方向上直接拦下。

// readKeySourcesPlatform 读主密钥的持久值来源：注册表已保存值
// （环境变量那侧由 KeySource.Read 统一补上）。
func readKeySourcesPlatform() KeySources {
	return KeySources{
		Persisted:      readPersistedKey(),
		PersistedWhere: `注册表 HKCU\Environment`,
	}
}

// reloadHint 让「本次运行实际使用」与「本机已保存」一致的推荐做法。
func reloadHint() string {
	return "重新打开终端（setx 写入的值只对新开的终端生效）"
}

// persistHint 把「本次运行这把密钥」写成持久值的做法（%SSHFLEET_KEY% 由当前终端展开，不必手打密钥）。
func persistHint() string {
	return fmt.Sprintf("在当前终端执行 setx %s \"%%%s%%\" 后重新打开终端", envName, envName)
}

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
	return strings.TrimSpace(val)
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
	fmt.Fprintf(out, "主密钥已%s，并写入本机（HKCU\\Environment）\n", actionDesc)
	fmt.Fprintln(out, "请重新打开终端后再使用：当前终端的环境变量仍是旧密钥")
	fmt.Fprintf(out, "%s注意：在此之前，本次运行读到的仍是旧密钥，执行 --convert-password 会被拦下（避免加密出将来看不开的文件）%s\n",
		colorYellow, colorReset)
	return nil
}
