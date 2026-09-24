// 归档：history/<YYYY-MM-DD_HH-MM-SS>_<模式>[_备注]/  ← 本次执行的全部产物
//
//	├── SSHFleetExec.log     执行日志（本次运行的节点级明细）
//	├── output.txt           终端输出（txt）
//	├── report.txt           统计报告
//	├── output.xlsx / results.xlsx   开关控制
//	└── assets/              清单与脚本的备份（清单为脱敏副本，不含上传文件）
//
// 工具日志不在这里，它是 history/SSHFleetTools.log 单一滚动文件。
package output

import (
	"encoding/csv"
	"fmt"
	"net/netip"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"sshfleet/internal/cli"
	"sshfleet/internal/common"
	"sshfleet/internal/config"
)

// Archive 一次执行的归档目录（执行期日志由 internal/log.InitExec 落盘，不经此处）。
type Archive struct {
	Dir string
}

// CreateArchive 创建归档目录。
func CreateArchive(cfg *config.Config, a *cli.Args) (*Archive, error) {
	dir, err := archiveDirName(cfg, a)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("创建归档目录失败：%s\n原因：%v", dir, err)
	}
	return &Archive{Dir: dir}, nil
}

// BackupAssets 把清单与脚本复制到 assets/（不备份上传文件）。
//
// 清单存的是**脱敏副本**：明文密码就写在清单第 4 列，原样复制等于把凭据又存了一份。
// 脚本没有凭据，原样复制。
func (ar *Archive) BackupAssets(cfg *config.Config, a *cli.Args) error {
	if ar == nil {
		return nil
	}
	assetsDir := filepath.Join(ar.Dir, config.BuiltinPaths.Asset)
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
		if err := copyCredentialList(a.CsvFile, assetsDir); err != nil {
			return err
		}
	}
	return nil
}

// copyCredentialList 把清单按脱敏副本写入 assets：每条记录的第 4 列（密码）与第 6 列
// （私钥口令）换成「前 2 + **** + 后 2」，其余字段与注释行原样保留。
func copyCredentialList(src, assetsDir string) error {
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

	var out strings.Builder
	for _, line := range strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			out.WriteString(line + "\n")
			continue
		}
		fields, perr := splitCSVLine(line)
		if perr != nil || len(fields) < 4 || !isIPv4Literal(fields[0]) {
			out.WriteString(line + "\n") // 表头、注释、异常行一律原样
			continue
		}
		for _, idx := range []int{3, 5} {
			if idx < len(fields) && strings.TrimSpace(fields[idx]) != "" {
				fields[idx] = common.MaskSecret(fields[idx])
			}
		}
		out.WriteString(joinCSVLine(fields))
	}
	return os.WriteFile(filepath.Join(assetsDir, filepath.Base(src)), []byte(out.String()), info.Mode().Perm())
}

// splitCSVLine 按 CSV 规则拆一行（容忍引号内的逗号）。
func splitCSVLine(line string) ([]string, error) {
	r := csv.NewReader(strings.NewReader(line))
	r.FieldsPerRecord = -1
	r.LazyQuotes = true
	return r.Read()
}

// isIPv4Literal 首列是不是 IPv4 字面量（清单的每一行都以 IP 开头，表头不是）。
func isIPv4Literal(s string) bool {
	addr, err := netip.ParseAddr(strings.TrimSpace(s))
	return err == nil && addr.Is4()
}

// joinCSVLine 按 CSV 规则拼一行。
func joinCSVLine(fields []string) string {
	var b strings.Builder
	w := csv.NewWriter(&b)
	_ = w.Write(fields)
	w.Flush()
	return b.String()
}

// CreateLatestHistoryLink 在当前目录建 latest_history 目录链接，指向最新归档目录。
// POSIX 用软链接，Windows 用目录联接（junction）——两者的取舍与限制见 createDirLink。
func CreateLatestHistoryLink(cfg *config.Config) error {
	entries, err := os.ReadDir(config.BuiltinPaths.Historys)
	if err != nil {
		return fmt.Errorf("读取历史记录目录失败：%s\n原因：%v", config.BuiltinPaths.Historys, err)
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
	latest := absoluteOrSelf(filepath.Join(config.BuiltinPaths.Historys, dirs[len(dirs)-1]))

	const linkName = "latest_history"
	if _, linked := linkTarget(linkName); linked {
		// 已是指向别处的链接：先删再建。软链接与联接点都能安全删除（只删链接本身，
		// 不会动到目标目录）。
		if err := os.Remove(linkName); err != nil {
			return fmt.Errorf("清理旧的 %s 失败：%v", linkName, err)
		}
	} else if _, err := os.Lstat(linkName); err == nil {
		fmt.Fprintf(os.Stderr, "%s[警告]%s 当前目录已存在同名文件 %s，跳过链接创建（如需快捷入口，删除该文件后重跑）\n", ansiYellow, ansiReset, linkName)
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
// 模式名由 cli.Args.ModeName 单点判定，此处不再重判 Args 的字段。
func archiveDirName(cfg *config.Config, a *cli.Args) (string, error) {
	modeName := a.ModeName()
	if modeName == "" {
		modeName = "unknown" // 防御：参数合规检查已保证四者必有其一
	}
	name := fmt.Sprintf("%s_%s", time.Now().Format("2006-01-02_15-04-05"), modeName)
	if remark := strings.TrimSpace(a.Remark); remark != "" {
		name += "_" + remark
	}
	return filepath.Join(config.BuiltinPaths.Historys, name), nil
}
