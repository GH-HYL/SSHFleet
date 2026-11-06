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

	// 两张表格：与终端文本、报告同族，都在本次目录里。名字照老版（用户认这两个名字）。
	// 导出给报告站：「生成哪张表失败了」那句话要印文件名，名字只许有这一处出处。
	NameOutputXlsx  = "terminal-output.xlsx"
	NameResultsXlsx = "results.xlsx"

	// 工具日志是**整个历史根共用的一份**滚动文件，不进本次目录——老版横跨所有运行、
	// 到 50MB 才换名留档，所以它与上面四个不同族：那四个在本次目录里，这一个在根上。
	nameToolLog = "SSHFleetTools.log"
)

// Paths 是本次运行的落点。**只有字符串**：目录还是文件、建没建，由要写的人自己管。
type Paths struct {
	Dir      string // 本次归档目录
	Assets   string // 目录里的 assets/（备份原料用，落地随阶段 07）
	Report   string // report.txt
	Terminal string // terminal-output.txt（执行站边跑边写）
	ExecLog  string // SSHFleetExec.log（执行期日志）

	// 两张表格的落点（阶段 07）：报告站收尾时写，开关关掉就不写。
	OutputXlsx  string
	ResultsXlsx string
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
		name += "_" + SanitizeRemark(r)
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

		OutputXlsx:  filepath.Join(dir, NameOutputXlsx),
		ResultsXlsx: filepath.Join(dir, NameResultsXlsx),
	}, nil
}

// ToolLog 报工具日志的两格落点：目录（历史根，调用方给）与文件名。
//
// 两格分开给，是因为 `log.Init` 收的就是这两格——它的签名因此不变，换掉的只是来源：
// 这条路径不再由日志包自己算（ADR-0032 第四条：路径只能从本包拿）。
//
// 目录原样交回、不换成绝对路径：日志自己那一行记的是「history」这个相对名（老版同），
// 换成绝对路径就改了那一行的样子。要绝对路径的是屏幕上那一行，见 `ToolLogPath`。
func ToolLog(root string) (dir, file string) {
	return root, nameToolLog
}

// ToolLogPath 报同一份日志的完整路径（绝对路径）。
//
// 起不来那一屏末尾的「日志：…」向它要——那一行是给人照着去找文件的，相对路径在别的
// 工作目录下就找不着。与 `ToolLog` 同一处拼，不另写一份。
func ToolLogPath(root string) string {
	dir, file := ToolLog(root)
	return absolute(filepath.Join(dir, file))
}

// absolute 把相对路径换成本机绝对路径；换不成时原样交回。
func absolute(p string) string {
	if abs, err := filepath.Abs(p); err == nil {
		return abs
	}
	return p
}
