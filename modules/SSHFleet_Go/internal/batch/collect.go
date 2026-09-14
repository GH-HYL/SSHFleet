package batch

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"sshfleet/internal/ssh"
)

// CollectResult 本地上传清单的采集结果。
type CollectResult struct {
	Files   []ssh.LocalFile // 可上传的真实文件（链接类条目已过滤）
	Skipped []string        // 被过滤条目的相对路径（POSIX 斜杠；仅用于提示，不上传）
}

// CollectLocalFiles 收集待上传的本地文件清单（对位旧 localfs.CollectFiles）：
// **链接类条目一律过滤**——软链接、Windows 目录联接、`.lnk` 快捷方式；它们记入 Skipped，
// 既不上传也不报错（用户 2026-09-14 裁定）。其余规则不变：拒 FIFO/device/socket、
// 验证可读性、目录递归。
//
// 每条带**相对上传根的路径**（POSIX 斜杠），上传侧据此保留目录层级（spec D49）：
// 上传根自身那一层不含在内（`-u modules -p /tmp/` → `/tmp/<contents>`），单文件即文件名。
//
// 只有过滤后**一个可传文件都不剩**时才报错——这是「过滤」语义的必然结果。
func CollectLocalFiles(root string) (*CollectResult, error) {
	if !filepath.IsAbs(root) {
		return nil, fmt.Errorf("file_path 必须是绝对路径: %s", root)
	}

	fi, err := os.Lstat(root)
	if err != nil {
		return nil, fmt.Errorf("file_path 不存在或无法访问: %w", err)
	}

	res := &CollectResult{}

	// 单文件：根路径本身是链接时同样过滤（没有可传目标 → 报错）
	if !fi.IsDir() {
		if isLinkish(root, fi) {
			return nil, fmt.Errorf("没有可上传的文件：-u 指定的路径本身是软链接/快捷方式：%s", root)
		}
		res.Files = append(res.Files, ssh.LocalFile{Path: root, Rel: fi.Name(), Size: fi.Size()})
		return res, nil
	}

	err = filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if p == root {
			return nil
		}
		// 链接类：软链接 / Windows 目录联接 / .lnk 快捷方式 —— 过滤，不报错
		if isLinkish(p, info) || strings.HasSuffix(strings.ToLower(info.Name()), ".lnk") {
			res.Skipped = append(res.Skipped, relOf(root, p))
			return nil
		}
		// FIFO / 设备 / socket：不是"链接"，过滤掉会静默改变语义，仍按错误处理
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

		res.Files = append(res.Files, ssh.LocalFile{Path: p, Rel: relOf(root, p), Size: info.Size()})
		return nil
	})
	if err != nil {
		return nil, err
	}
	if len(res.Files) == 0 {
		return nil, fmt.Errorf("没有可上传的文件：%s 下的文件全部是软链接/快捷方式（已过滤 %d 个：%s）",
			root, len(res.Skipped), Summarize(res.Skipped))
	}
	return res, nil
}

// isLinkish 判断目录项是否为「链接类」（软链接 / Windows 目录联接）。
// 注意：Go 1.23 起 Windows 目录联接被标为 ModeIrregular 而**不是** ModeSymlink
// （同 spec D47 的踩坑），故 ModeIrregular 时用 Readlink 复核一次，避免误判其它非常规文件。
func isLinkish(path string, info os.FileInfo) bool {
	if info.Mode()&os.ModeSymlink != 0 {
		return true
	}
	if info.Mode()&os.ModeIrregular != 0 {
		_, err := os.Readlink(path)
		return err == nil
	}
	return false
}

// relOf 相对上传根的路径（POSIX 斜杠，远端拼接用）。
func relOf(root, p string) string {
	rel, err := filepath.Rel(root, p)
	if err != nil {
		return filepath.ToSlash(p)
	}
	return filepath.ToSlash(rel)
}

// Summarize 把被过滤的条目拼成提示文案（超过 5 个时截断）。
func Summarize(names []string) string {
	const maxShow = 5
	if len(names) <= maxShow {
		return strings.Join(names, "、")
	}
	return strings.Join(names[:maxShow], "、") + fmt.Sprintf(" 等 %d 个", len(names))
}
