// 归档（对位旧 logger.create_exec_log_dir + archive.py + 软链接）：
//
//	historys/<YYYY-MM-DD_HH-MM-SS>_<模式>[_备注]/  ← 本次执行的全部产物
//	├── <paths.exec>      执行日志（本次运行的节点级明细）
//	├── <paths.output>    终端输出（txt）
//	├── <paths.report>    统计报告
//	├── <paths.output_xlsx> / <paths.results_xlsx>   开关控制
//	└── <paths.asset>/    资源备份（清单与脚本，spec D38：不含上传文件）
//
// 工具日志不在这里，它是 historys/<paths.tool> 单一滚动文件（spec D36 / M5-3 裁定）。
package output

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"sshfleet/internal/cli"
	"sshfleet/internal/config"
)

// Archive 一次执行的归档目录（持有执行日志文件句柄）。
type Archive struct {
	Dir      string
	execFile *os.File
	execPath string
}

// CreateArchive 创建归档目录与执行日志文件。
func CreateArchive(cfg *config.Config, a *cli.Args) (*Archive, error) {
	dir, err := archiveDirName(cfg, a)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("创建归档目录失败：%s\n原因：%v", dir, err)
	}

	execPath := filepath.Join(dir, cfg.Paths.Exec)
	f, err := os.Create(execPath)
	if err != nil {
		return nil, fmt.Errorf("创建执行日志失败：%s\n原因：%v", execPath, err)
	}
	return &Archive{Dir: dir, execFile: f, execPath: execPath}, nil
}

// ExecWriter 执行日志写入器（危险放行留痕、节点明细都写这里）。
func (ar *Archive) ExecWriter() io.Writer {
	if ar == nil {
		return io.Discard
	}
	return ar.execFile
}

// ExecLog 写一行执行日志。
func (ar *Archive) ExecLog(format string, args ...any) error {
	if ar == nil || ar.execFile == nil {
		return nil
	}
	_, err := fmt.Fprintf(ar.execFile, format+"\n", args...)
	return err
}

// Close 关闭执行日志。
func (ar *Archive) Close() error {
	if ar == nil || ar.execFile == nil {
		return nil
	}
	err := ar.execFile.Close()
	ar.execFile = nil
	return err
}

// BackupAssets 把清单与脚本复制到 assets/（spec D38：不备份上传文件）。
func (ar *Archive) BackupAssets(cfg *config.Config, a *cli.Args) error {
	if ar == nil {
		return nil
	}
	assetsDir := filepath.Join(ar.Dir, cfg.Paths.Asset)
	if err := os.MkdirAll(assetsDir, 0o755); err != nil {
		return err
	}
	copyFile := func(src string) error {
		if src == "" {
			return nil
		}
		info, err := os.Stat(src)
		if err != nil || info.IsDir() {
			return nil
		}
		data, err := os.ReadFile(src)
		if err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(assetsDir, filepath.Base(src)), data, info.Mode().Perm())
	}
	if err := copyFile(a.Script); err != nil {
		return err
	}
	// 内联清单没有文件可备份（-f 为内联文本时跳过）
	if !a.FIsInline {
		if err := copyFile(a.CsvFile); err != nil {
			return err
		}
	}
	return nil
}

// CreateLatestHistoryLink 在当前目录建 latest_history 目录链接，指向最新归档目录。
// POSIX 用软链接，Windows 用目录联接（junction）——两者的取舍与限制见 createDirLink。
func CreateLatestHistoryLink(cfg *config.Config) error {
	entries, err := os.ReadDir(cfg.Paths.Historys)
	if err != nil {
		return fmt.Errorf("读取历史记录目录失败：%s\n原因：%v", cfg.Paths.Historys, err)
	}
	var dirs []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dirs = append(dirs, e.Name())
	}
	if len(dirs) == 0 {
		return nil
	}
	// 目录名以时间开头，字典序即时间序
	sort.Strings(dirs)
	latest := absoluteOrSelf(filepath.Join(cfg.Paths.Historys, dirs[len(dirs)-1]))

	const linkName = "latest_history"
	if _, linked := linkTarget(linkName); linked {
		// 已是指向别处的链接：先删再建。软链接与联接点都能安全删除（只删链接本身，
		// 不会动到目标目录）。
		if err := os.Remove(linkName); err != nil {
			return fmt.Errorf("清理旧的 %s 失败：%v", linkName, err)
		}
	} else if _, err := os.Lstat(linkName); err == nil {
		fmt.Printf("警告: 当前目录已存在同名文件 %s，跳过链接创建（如需快捷入口，删除该文件后重跑）\n", linkName)
		return nil
	}
	if err := createDirLink(linkName, latest); err != nil {
		return fmt.Errorf("创建 latest_history 目录链接失败：%v\n提示：Windows 使用目录联接（需目标位于本地 NTFS 卷），POSIX 使用符号链接", err)
	}
	return nil
}

// linkTarget 判断 path 是否为目录链接（POSIX 软链接 / Windows 目录联接），是则返回其指向。
// 统一用 os.Readlink 作判据：它对普通文件与普通目录一律失败，对重解析点成功。
// 不可只判 os.ModeSymlink——Go 1.23 起 junction（MOUNT_POINT）不再被标为 ModeSymlink，
// 而是 ModeIrregular（见 os/types_windows.go 的 fs.mode）。
func linkTarget(path string) (string, bool) {
	target, err := os.Readlink(path)
	if err != nil {
		return "", false
	}
	return target, true
}

func absoluteOrSelf(p string) string {
	abs, err := filepath.Abs(p)
	if err != nil {
		return p
	}
	return abs
}

// archiveDirName 归档目录名：<时间>_<模式>[_备注]（对位旧命名）。
func archiveDirName(cfg *config.Config, a *cli.Args) (string, error) {
	modeName := "unknown"
	switch {
	case a.Command != "":
		modeName = "command"
	case a.Script != "":
		modeName = "script"
	case a.Upload != "":
		modeName = "upload"
	case a.Download != "":
		modeName = "download"
	}
	name := fmt.Sprintf("%s_%s", time.Now().Format("2006-01-02_15-04-05"), modeName)
	if remark := strings.TrimSpace(a.Remark); remark != "" {
		name += "_" + remark
	}
	return filepath.Join(cfg.Paths.Historys, name), nil
}
