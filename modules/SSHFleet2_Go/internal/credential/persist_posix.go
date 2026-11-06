//go:build !windows

package credential

import (
	"os"
	"path/filepath"
	"strings"

	"sshfleet2/internal/contract"
)

// POSIX 侧主密钥持久化：写登录 shell 的 rc 文件。
//
// 选哪个文件按 `$SHELL`：含 zsh 用 `.zshrc`，其余用 `.bashrc`（老版同一条，修的是更早版本
// 硬编码 bashrc 造成的「生成成功、下条命令仍缺密钥」）。

// hostPlatform 说持久值落在哪一侧：拼话按它挑「source 一下」还是「重开终端」。
const hostPlatform = contract.KeyHostPOSIX

// rcFiles 是两个候选：第一个有主密钥的就是「本机已保存」，两个不同就是「另一处持久值」。
var rcFiles = []string{".zshrc", ".bashrc"}

// homeDir 取主目录；取不到就交回空串（那时所有路径都算不出来，读写都不会发生）。
func homeDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return home
}

// currentRCFile 是当前登录 shell 对应的 rc 文件名。
func currentRCFile() string {
	if strings.Contains(os.Getenv("SHELL"), "zsh") {
		return ".zshrc"
	}
	return ".bashrc"
}

// rcFilePath 是当前 shell 的 rc 文件绝对路径；取不到主目录时退回文件名。
func rcFilePath() string {
	home := homeDir()
	if home == "" {
		return currentRCFile()
	}
	return filepath.Join(home, currentRCFile())
}

// readSourcesPlatform 读两个 rc 文件里的 export 行。
//
// 环境变量那一侧由 `KeySource.Read` 统一补上，这里只管持久值。
func readSourcesPlatform() Sources {
	var src Sources
	home := homeDir()
	if home == "" {
		return src
	}
	for _, rc := range rcFiles {
		path := filepath.Join(home, rc)
		key := findExportInFile(path)
		if strings.TrimSpace(key) == "" {
			continue
		}
		if src.Persisted == "" {
			src.Persisted, src.PersistedWhere = key, path
			continue
		}
		if key != src.Persisted {
			src.OtherKey, src.OtherWhere = key, path
		}
	}
	return src
}

// persistKey 把主密钥写进当前 shell 的 rc 文件，另一侧若已存着别的主密钥一并改成同一把。
//
// 另一侧**不存在或没有主密钥行**时不新建它：使用者的主目录里凭空多出一个 rc 文件，
// 比密钥没同步更让人意外。当前 shell 那一侧照老版新建——空主目录上 `--gen-key` 总得
// 有个地方落，否则这条命令等于没写成。
func persistKey(key string) (contract.PersistResult, error) {
	home := homeDir()
	if home == "" {
		return contract.PersistResult{}, os.ErrNotExist
	}
	rc := currentRCFile()
	target := filepath.Join(home, rc)
	if err := writeKeyToRC(target, key); err != nil {
		return contract.PersistResult{Where: target}, err
	}
	res := contract.PersistResult{Where: target}

	for _, other := range rcFiles {
		if other == rc {
			continue
		}
		otherPath := filepath.Join(home, other)
		if !hasKeyLine(otherPath) {
			continue
		}
		if err := writeKeyToRC(otherPath, key); err != nil {
			return contract.PersistResult{Where: otherPath}, err
		}
		res.OtherUnified = otherPath
	}
	return res, nil
}
