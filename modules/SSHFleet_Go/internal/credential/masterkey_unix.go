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
// rc 文件里主密钥行的读写原语在 rcfile.go（平台无关，便于单测）。

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

// persistKey 把主密钥写进登录 shell 的 rc 文件（按 $SHELL 选 zsh/bash，D30）：
// 目标 rc 内的主密钥行收敛为末尾唯一一条；另一侧 rc 文件若**已存在且已含**主密钥行，
// 一并改写为同一把钥匙（两个 shell 各持不同密钥是同一类分叉，只会在换 shell 后爆发）。
// 不存在的 rc 文件不会被创建；写完提示 source 或重开终端生效。
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
	if err := writeKeyToRC(rcPath, key); err != nil {
		return fmt.Errorf("主密钥自动保存失败：%v\n可手动保存：在 %s 追加 export %s='你的随机密钥'", err, rcPath, envName)
	}
	fmt.Fprintf(out, "主密钥已%s，并自动保存到 %s\n", actionDesc, rcPath)

	for _, other := range rcFiles {
		if other == rc || !hasKeyLine(filepath.Join(home, other)) {
			continue
		}
		otherPath := filepath.Join(home, other)
		if err := writeKeyToRC(otherPath, key); err != nil {
			return fmt.Errorf("主密钥自动保存失败：%v\n可手动保存：在 %s 追加 export %s='你的随机密钥'", err, otherPath, envName)
		}
		fmt.Fprintf(out, "另一侧 shell 的 %s 原本存着别的主密钥，已统一为同一把\n", otherPath)
	}

	fmt.Fprintf(out, "请执行 source %s 或重新打开终端后生效\n", rcPath)
	return nil
}
