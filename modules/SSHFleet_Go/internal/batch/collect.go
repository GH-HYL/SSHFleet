package batch

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"sshfleet/internal/ssh"
)

// CollectLocalFiles 收集待上传的本地文件清单（对位旧 localfs.CollectFiles）：
// 拒软链接与 .lnk、拒 FIFO/device/socket、验证可读性、目录递归，
// 且**只取文件名**（扁平化上传，不保留子目录结构——旧行为）。
func CollectLocalFiles(root string) ([]ssh.LocalFile, error) {
	if !filepath.IsAbs(root) {
		return nil, fmt.Errorf("file_path 必须是绝对路径: %s", root)
	}

	fi, err := os.Lstat(root)
	if err != nil {
		return nil, fmt.Errorf("file_path 不存在或无法访问: %w", err)
	}

	if !fi.IsDir() {
		if fi.Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("file_path 是软链接: %s", root)
		}
		return []ssh.LocalFile{{Path: root, Name: fi.Name(), Size: fi.Size()}}, nil
	}

	var items []ssh.LocalFile
	err = filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if p == root {
			return nil
		}
		// 软链接（文件与目录都跳过）
		if info.Mode()&os.ModeSymlink != 0 {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		// Windows .lnk 快捷方式跳过
		if strings.HasSuffix(strings.ToLower(info.Name()), ".lnk") {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		// FIFO / 设备 / socket 不支持
		if info.Mode()&os.ModeNamedPipe != 0 || info.Mode()&os.ModeDevice != 0 || info.Mode()&os.ModeSocket != 0 {
			return fmt.Errorf("不支持的文件类型: %s (FIFO/device/socket)", p)
		}
		if info.IsDir() {
			return nil
		}
		// 验证可读性
		f, oerr := os.Open(p)
		if oerr != nil {
			return fmt.Errorf("文件不可读: %s: %w", p, oerr)
		}
		_ = f.Close()
		items = append(items, ssh.LocalFile{Path: p, Name: info.Name(), Size: info.Size()})
		return nil
	})
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("目录中没有真实文件: %s", root)
	}
	return items, nil
}
