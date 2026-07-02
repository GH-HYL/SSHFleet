package localfs

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// FileItem 单个文件信息
type FileItem struct {
	LocalPath string // 绝对路径
	RelPath   string // 相对于 file_path 的相对路径
	FileName  string // 文件名
	FileSize  int64
}

// CollectFiles 收集文件清单，过滤软链接和快捷方式
func CollectFiles(filePath string) ([]FileItem, error) {
	// 必须是绝对路径
	if !filepath.IsAbs(filePath) {
		return nil, fmt.Errorf("file_path 必须是绝对路径: %s", filePath)
	}

	fi, err := os.Stat(filePath)
	if err != nil {
		return nil, fmt.Errorf("file_path 不存在或无法访问: %w", err)
	}

	// 单文件
	if !fi.IsDir() {
		if fi.Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("file_path 是软链接: %s", filePath)
		}
		item, err := newFileItem(filePath, filePath, fi)
		if err != nil {
			return nil, err
		}
		return []FileItem{item}, nil
	}

	// 目录递归
	var items []FileItem
	err = filepath.Walk(filePath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return fmt.Errorf("遍历文件失败 %s: %w", path, err)
		}

		// 跳过根目录自身
		if path == filePath {
			return nil
		}

		// 跳过软链接（文件和目录都跳过）
		if info.Mode()&os.ModeSymlink != 0 {
			return filepath.SkipDir
		}

		// Windows .lnk 快捷方式跳过
		if strings.HasSuffix(strings.ToLower(info.Name()), ".lnk") {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		// 命名管道、设备文件、socket 报错
		if info.Mode()&os.ModeNamedPipe != 0 ||
			info.Mode()&os.ModeDevice != 0 ||
			info.Mode()&os.ModeSocket != 0 {
			return fmt.Errorf("不支持的文件类型: %s (FIFO/device/socket)", path)
		}

		// 目录：跳过（不收集目录本身，只收集文件）
		if info.IsDir() {
			return nil
		}

		// 尝试读取文件，验证可读性
		f, err := os.Open(path)
		if err != nil {
			return fmt.Errorf("文件不可读: %s: %w", path, err)
		}
		f.Close()

		item, err := newFileItem(filePath, path, info)
		if err != nil {
			return err
		}
		items = append(items, item)
		return nil
	})

	if err != nil {
		return nil, err
	}

	if len(items) == 0 {
		return nil, fmt.Errorf("目录中没有真实文件: %s", filePath)
	}

	return items, nil
}

func newFileItem(basePath, fullPath string, fi os.FileInfo) (FileItem, error) {
	relPath, err := filepath.Rel(basePath, fullPath)
	if err != nil {
		return FileItem{}, fmt.Errorf("计算相对路径失败: %w", err)
	}

	return FileItem{
		LocalPath: fullPath,
		RelPath:   filepath.ToSlash(relPath),
		FileName:  fi.Name(),
		FileSize:  fi.Size(),
	}, nil
}
