//go:build !windows

package archive

import "os"

// createDirLink 在 POSIX 上建目录软链接。
//
// Windows 那一份见 link_windows.go——两边都要能当目录用（`cd latest_history` 进得去），
// 但手段不同：那边不用符号链接，因为它要管理员权限或开发者模式。
func createDirLink(link, target string) error {
	return os.Symlink(target, link)
}
