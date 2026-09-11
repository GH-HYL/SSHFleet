package credential

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"golang.org/x/sys/windows/registry"

	"sshfleet/internal/common"
)

// 主密钥管理（对位旧 master_key.py）：
//   - 运行期从环境变量 SSHFLEET_KEY 读取（spec D15 统一名）
//   - Windows：读取走注册表直读（实现途径 7，不起 reg 子进程）；写入维持 setx
//   - Linux/macOS：检测登录 shell，zsh → ~/.zshrc、bash/其他 → ~/.bashrc（spec D30）

const envName = "SSHFLEET_KEY"

// GetMasterKey 从环境变量读取主密钥；缺失时返回带生成指引的错误。
func GetMasterKey() (string, error) {
	key := strings.TrimSpace(os.Getenv(envName))
	if key == "" {
		return "", fmt.Errorf("缺少主密钥，无法解密/加密凭据文件\n请先生成主密钥：SSHFleet --gen-key")
	}
	return key, nil
}

// readPersistedKey 从持久化位置读取已存主密钥（Windows: 注册表；Unix: ~/.zshrc 与 ~/.bashrc 都查），无则返回空串。
func readPersistedKey() string {
	if runtime.GOOS == "windows" {
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
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	for _, rc := range []string{".zshrc", ".bashrc"} {
		if key := findExportInFile(filepath.Join(home, rc)); key != "" {
			return key
		}
	}
	return ""
}

func findExportInFile(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	needle := "export " + envName + "="
	for _, line := range strings.Split(string(data), "\n") {
		if strings.Contains(line, needle) {
			// 对位旧正则 export SSHFLEET_KEY=['"]?([^'"\s]+)：取引号或空白前的值
			rest := line[strings.Index(line, needle)+len(needle):]
			rest = strings.TrimLeft(rest, "'\"")
			if i := strings.IndexAny(rest, "'\"\t "); i >= 0 {
				rest = rest[:i]
			}
			return rest
		}
	}
	return ""
}

// persistKey 把主密钥持久化：Windows setx 写注册表；Unix 追加/替换登录 shell 的 rc 文件。
func persistKey(key string, regenerated bool, out *strings.Builder) error {
	actionDesc := "生成"
	if regenerated {
		actionDesc = "重新生成"
	}
	if runtime.GOOS == "windows" {
		cmd := exec.Command("setx", envName, key)
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("主密钥自动保存失败：%v\n可手动保存：执行 setx %s 你的随机密钥", err, envName)
		}
		fmt.Fprintf(out, "主密钥已%s，并自动保存到本机\n", actionDesc)
		fmt.Fprintln(out, "请重新打开终端后再使用（当前终端读不到新密钥）")
		return nil
	}

	rc := ".bashrc"
	if strings.Contains(os.Getenv("SHELL"), "zsh") {
		rc = ".zshrc" // D30：zsh 写 ~/.zshrc，修复旧版硬编码 bashrc 导致的"生成成功但下条命令仍缺密钥"
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	rcPath := filepath.Join(home, rc)
	exportLine := "export " + envName + "='" + key + "'"

	var lines []string
	if data, err := os.ReadFile(rcPath); err == nil {
		lines = strings.Split(string(data), "\n")
	}
	replaced := false
	for idx, line := range lines {
		if strings.Contains(line, "export "+envName+"=") {
			lines[idx] = exportLine
			replaced = true
			break
		}
	}
	if !replaced {
		if len(lines) > 0 && lines[len(lines)-1] != "" {
			lines = append(lines, "")
		}
		lines = append(lines, "# SSHFleet 主密钥", exportLine)
	}
	content := strings.Join(lines, "\n")
	if err := os.WriteFile(rcPath, []byte(content), 0o644); err != nil {
		return fmt.Errorf("主密钥自动保存失败：%v\n可手动保存：在 %s 追加 export %s='你的随机密钥'", err, rcPath, envName)
	}
	fmt.Fprintf(out, "主密钥已%s，并自动保存到 %s\n", actionDesc, rcPath)
	fmt.Fprintf(out, "请执行 source %s 或重新打开终端后生效\n", rcPath)
	return nil
}

// GenKey 处理 --gen-key：生成随机主密钥并持久化；已有密钥时先确认覆盖，
// 非交互模式拒绝自动覆盖（覆盖后旧密钥加密的文件无法解密）。
func GenKey(in *common.Interactor) error {
	newKey, err := GenerateMasterKey()
	if err != nil {
		return err
	}
	envKey := strings.TrimSpace(os.Getenv(envName))
	persistedKey := readPersistedKey()

	overwritten := false
	if envKey != "" || persistedKey != "" {
		fmt.Fprintln(in.Out, "检测到已存在主密钥，当前未做任何修改")
		fmt.Fprintln(in.Out, "注意：如果覆盖，用旧密钥加密的凭据文件将永久无法解密")
		if in.Disinteractive {
			return fmt.Errorf("检测到已存在主密钥，非交互模式不自动覆盖（覆盖后旧密钥加密的凭据文件将无法解密）。\n如需覆盖：去掉 --disinteractive 后重新执行 --gen-key")
		}
		confirmed, err := in.Confirm("是否确认覆盖？", false)
		if err != nil {
			// 对位旧 _confirm_overwrite：EOF 视为「否」，保留原密钥正常返回（不作为取消）
			fmt.Fprintln(in.Out, "已保留原密钥，未做修改")
			return nil
		}
		if !confirmed {
			fmt.Fprintln(in.Out, "已保留原密钥，未做修改")
			return nil
		}
		overwritten = true
		fmt.Fprintln(in.Out, "已确认覆盖，正在重新生成主密钥...")
	}

	var out strings.Builder
	if err := persistKey(newKey, overwritten, &out); err != nil {
		return err
	}
	fmt.Fprint(in.Out, out.String())
	return nil
}
