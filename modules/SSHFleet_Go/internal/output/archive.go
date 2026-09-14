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
	"runtime"
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

// CreateLatestHistoryLink 在当前目录建 latest_history 软链接指向最新归档目录（POSIX only）。
// Windows 上创建符号链接需要开发者模式/管理员权限，旧实现多次尝试未成——此处同样跳过并提示。
func CreateLatestHistoryLink(cfg *config.Config) error {
	if runtime.GOOS == "windows" {
		fmt.Println("提示：当前系统为 Windows，符号链接需要开发者模式或管理员权限，已跳过 latest_history 创建")
		return nil
	}

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
	latest := filepath.Join(cfg.Paths.Historys, dirs[len(dirs)-1])

	const linkName = "latest_history"
	if info, err := os.Lstat(linkName); err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			if err := os.Remove(linkName); err != nil {
				return err
			}
		} else {
			fmt.Printf("警告: 当前目录已存在同名文件 %s，跳过符号链接创建（如需快捷入口，删除该文件后重跑）\n", linkName)
			return nil
		}
	}
	if err := os.Symlink(absoluteOrSelf(latest), linkName); err != nil {
		return fmt.Errorf("创建 latest_history 符号链接失败：%v", err)
	}
	return nil
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
