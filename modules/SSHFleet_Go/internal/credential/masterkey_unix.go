//go:build !windows

package credential

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Unix 侧主密钥持久化：写登录 shell 的 rc 文件（spec D30，修复旧版硬编码 bashrc
// 导致的「生成成功但下条命令仍缺密钥」）。

// rcFiles 查找顺序：zsh 在前，与旧实现一致。
var rcFiles = []string{".zshrc", ".bashrc"}

// readPersistedKey 依次查 ~/.zshrc 与 ~/.bashrc，命中即返回；都没有返回空串。
func readPersistedKey() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	for _, rc := range rcFiles {
		if key := findExportInFile(filepath.Join(home, rc)); key != "" {
			return key
		}
	}
	return ""
}

// findExportInFile 在 rc 文件里找 export SSHFLEET_KEY=... 的值。
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

// persistKey 把主密钥写进登录 shell 的 rc 文件（按 $SHELL 选 zsh/bash，D30）：
// 已有 export 行则替换，否则追加；写完后提示 source 或重开终端生效。
func persistKey(key string, regenerated bool, out *strings.Builder) error {
	actionDesc := "生成"
	if regenerated {
		actionDesc = "重新生成"
	}

	rc := ".bashrc"
	if strings.Contains(os.Getenv("SHELL"), "zsh") {
		rc = ".zshrc"
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
