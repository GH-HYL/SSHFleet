package archive

// 「最新一次」那条入口：工作目录下的 `latest_history`，指向历史根里最新那一次的目录。
//
// 名字内置、不开放配置（与本次目录里那几个文件名同族）。

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// LatestName 是工作目录下那条入口的名字。
const LatestName = "latest_history"

// CreateLatestLink 在当前目录建（或重指）`latest_history`，指向历史根里最新那一次的目录。
//
// **落在当前目录**：老版就建在进程的工作目录下，本包沿用——`history` 那个历史根本来也是
// 相对名（`contract.HistoryDir`），两者同一套基准，不另引入「工作目录」这个参数。
//
// 判据照老版：
//   - 历史根里的目录名以时间开头，字典序即时间序，取最后一个；
//   - 已经是一条指向别处的链接：先删再建（只删链接本身，不动目标目录）；
//   - 当前目录里是**同名的普通文件**：那是使用者的东西，不删、不报错，交回 skipped 让上面
//     说一句「跳过创建」；
//   - 历史根空着：什么都不做。
//
// 交回的错误只说原因，前缀与提示由 `message` 那一句加——话不从本包出。
func CreateLatestLink(historyRoot string) (skipped bool, err error) {
	entries, err := os.ReadDir(historyRoot)
	if err != nil {
		return false, fmt.Errorf("读取历史记录目录 %s：%w", historyRoot, err)
	}
	var dirs []string
	for _, e := range entries {
		if e.IsDir() {
			dirs = append(dirs, e.Name())
		}
	}
	if len(dirs) == 0 {
		return false, nil
	}
	sort.Strings(dirs)
	latest, err := filepath.Abs(filepath.Join(historyRoot, dirs[len(dirs)-1]))
	if err != nil {
		return false, err
	}

	link := LatestName
	if _, isLink := linkTarget(link); isLink {
		if err := os.Remove(link); err != nil {
			return false, fmt.Errorf("清理旧的 %s：%w", LatestName, err)
		}
	} else if _, err := os.Lstat(link); err == nil {
		return true, nil // 同名的普通文件：不碰它
	}

	if err := createDirLink(link, latest); err != nil {
		return false, err
	}
	return false, nil
}

// linkTarget 判 path 是不是一条目录链接（POSIX 软链接 / Windows 目录联接），是则交出目标。
//
// 统一用 `os.Readlink` 作判据：它对普通文件与普通目录一律失败，对重解析点成功。不能只判
// `os.ModeSymlink`——Go 1.23 起 Windows 的目录联接不再被标成 `ModeSymlink`，而是
// `ModeIrregular`，只判那一格会漏。
func linkTarget(path string) (string, bool) {
	target, err := os.Readlink(path)
	if err != nil {
		return "", false
	}
	return target, true
}
