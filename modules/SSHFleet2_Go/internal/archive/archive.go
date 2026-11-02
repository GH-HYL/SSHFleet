// Package archive 是横切叶子包：建本次运行的落盘目录、发路径字符串。
//
// 不返回目录句柄（那是可变共享物），也不替别人写文件——谁产内容谁写。
//
// 判据见 ADR-0032 第四条。
package archive

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// 目录与文件名的内置取值（用户不可配）。
//
// 「history」不在这里：它是根的上一级，由塔尖把契约里那个常量递进来——同一个名字
// 只该有一处出处，而本包按权限表不依赖契约（ADR-0032 第四条）。
const (
	nameAssets   = "assets"
	nameReport   = "report.txt"
	nameTerminal = "terminal-output.txt"
	nameExecLog  = "SSHFleetExec.log"
)

// Paths 是本次运行的落点。**只有字符串**：目录还是文件、建没建，由要写的人自己管。
type Paths struct {
	Dir      string // 本次归档目录
	Assets   string // 目录里的 assets/（备份原料用，落地随阶段 07）
	Report   string // report.txt
	Terminal string // terminal-output.txt（执行站边跑边写）
	ExecLog  string // SSHFleetExec.log（执行期日志）
}

// Create 建本次的根与 assets/，交回那一组路径。
//
// root 是历史记录根目录（`history`），由调用方给——本包不记住它，也不设包级变量
// 记「这次建了哪个目录」：路径是算出来的，不是存下来的。
//
// 目录名照老版：`<yyyy-MM-dd_HH-mm-ss>_<模式>[_备注]`。备注过了平台过滤——显式 `-r`
// 与将来自动生成的那条都要过，只在自动生成那条路上过滤会漏掉显式 `-r`（老版 2026-10-15
// 实测：`-r a/b` 被内核当路径，归档目录平白多出一层）。
func Create(root, mode, remark string, now time.Time) (Paths, error) {
	if strings.TrimSpace(mode) == "" {
		mode = "unknown" // 防御：能走到这里的纸一定选定了模式
	}
	name := now.Format("2006-01-02_15-04-05") + "_" + mode
	if r := strings.TrimSpace(remark); r != "" {
		name += "_" + sanitizeRemark(r)
	}

	dir := filepath.Join(root, name)
	assets := filepath.Join(dir, nameAssets)
	if err := os.MkdirAll(assets, 0o755); err != nil {
		return Paths{}, fmt.Errorf("建归档目录失败（%s）：%w", dir, err)
	}

	return Paths{
		Dir:      dir,
		Assets:   assets,
		Report:   filepath.Join(dir, nameReport),
		Terminal: filepath.Join(dir, nameTerminal),
		ExecLog:  filepath.Join(dir, nameExecLog),
	}, nil
}
