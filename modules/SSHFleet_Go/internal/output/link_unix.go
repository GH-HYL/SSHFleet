//go:build !windows

package output

import "os"

// createDirLink 在 POSIX 上建目录软链接（对位旧实现）。
func createDirLink(link, target string) error {
	return os.Symlink(target, link)
}
