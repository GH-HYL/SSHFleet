// 运行期呈现器（单节点结果的三个去向：终端明细 / output.txt / 执行期日志）。
//
// 为什么住这里：一条结果的「分类 → 三去向」是一件事，原先整段编排住在 main 的闭包里，
// 连同并发敏感的进度界面懒加载（互斥锁 + nil 判断）一起。那让 main 承担了本该属于
// 呈现层的职责，也让这段行为完全无法在测试里复现（只能真跑一次 SSH）。
//
// 边界：本模块只做「呈现」，不控制生命周期——main 仍独占主干与退出权，
// batch 的 Hooks 形状不变，只是回调体从 main 的闭包变成这里的三个方法。
package output

import (
	"fmt"
	"io"
	"os"
	"sync"

	"sshfleet/internal/batch"
	"sshfleet/internal/log"
	"sshfleet/internal/result"
	"sshfleet/internal/ssh"
)

// Reporter 执行期呈现器：接住 batch 的三个事件（采集提示 / 进度 / 单节点结果），
// 把每一条结果送往它该去的地方。
//
// 接口（调用方需要知道的全部）：
//   - 用 NewReporter 构造，构造时一次性给出执行期日志、output.txt 句柄、分类依据与模式
//   - Notice / Progress / Result 三个方法与 batch.Hooks 的三个字段一一对应，可直接赋值
//   - 用完后调 Stop 收尾（进度界面落回界面下方），此后不应再收到事件
//
// 隐藏的实现：进度界面的**懒创建**（首个进度事件才建，好让采集期提示先落终端、
// 不被进度条的光标上移重绘吃掉——用户 2026-09-14 裁定）、运行期提示与单条明细
// 改从进度界面上方打印（对位旧 rich Live 的上滚动区 + 下进度条）、
// 三去向的分发与分类适配。
type Reporter struct {
	logger   *log.Logger
	outFile  io.Writer
	mode     string
	keyword  *result.Keywords
	total    int
	classify func(ssh.Result) string

	mu sync.Mutex
	ui *ProgressUI
}

// NewReporter 构造呈现器。
//   - execLog: 执行期日志（已轮转至归档目录；nil 表示不写）
//   - outputFile: output.txt 句柄（nil 表示不写）
//   - total: 节点总数（进度界面的分母，懒创建时用）
//
// 分类依据（模式 + 错误关键词）在这里收下，调用方不必再自己拼分类函数。
func NewReporter(execLog *log.Logger, outputFile io.Writer, mode string, total int, kw *result.Keywords) *Reporter {
	r := &Reporter{
		logger:  execLog,
		outFile: outputFile,
		mode:    mode,
		keyword: kw,
		total:   total,
	}
	r.classify = func(res ssh.Result) string {
		return result.Classify(result.Case{
			ExitCode:     res.ExitCode,
			Error:        deref(res.Error),
			Output:       res.Output,
			AuthFailure:  deref(res.AuthFailure),
			Mode:         mode,
			SuccessFiles: res.SuccessFiles,
			FailedFiles:  res.FailedFiles,
		}, kw)
	}
	return r
}

// Notice 采集期提示（目前仅上传源中被过滤的链接）：终端 + 执行期日志。
func (r *Reporter) Notice(msg string) {
	r.printAbove(msg)
	if r.logger != nil {
		r.logger.Info(msg)
	}
}

// Progress 进度事件：首个事件才创建进度界面（采集期提示已先落终端）。
func (r *Reporter) Progress(s batch.Snapshot) {
	r.mu.Lock()
	if r.ui == nil {
		r.ui = NewProgressUI(os.Stdout, r.mode, r.total)
	}
	ui := r.ui
	r.mu.Unlock()
	ui.Update(s)
}

// Result 单节点结果：写 output.txt（各模式）+ 命令模式打终端明细 + 写执行期日志。
func (r *Reporter) Result(res ssh.Result) {
	category := r.classify(res)

	// output.txt 只落文件：终端明细改经 printAbove（进度界面上方），
	// 与进度条各占一块区域、互不覆盖。
	_ = PrintResult(io.Discard, r.outFile, res, r.mode, category)

	if r.mode == "execute" {
		r.printAbove(ResultLine(res, r.mode, category))
	}
	r.logNode(res, category)
}

// Category 单条结果的分类名（xlsx 导出等收尾环节按同一口径取用，
// 避免调用方自己再拼一份分类适配）。
func (r *Reporter) Category(res ssh.Result) string { return r.classify(res) }

// Stop 收尾：进度界面落回界面下方，后续输出不再覆盖它。幂等。
func (r *Reporter) Stop() {
	r.mu.Lock()
	ui := r.ui
	r.mu.Unlock()
	if ui != nil {
		ui.Stop()
	}
}

// printAbove 在进度界面上方打印；界面尚未创建时直接落 stdout。
func (r *Reporter) printAbove(text string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.ui != nil {
		r.ui.PrintAbove(text)
		return
	}
	fmt.Fprintln(os.Stdout, text)
}

// logNode 把单节点结果按运行事件写入执行期日志（对位旧引擎的
// 「SSH连接成功/失败 → 执行结束/节点完成 → 分类」三级记录，全部带时间戳与级别）。
func (r *Reporter) logNode(res ssh.Result, category string) {
	el := r.logger
	if el == nil {
		return
	}
	ip := "【" + res.IP + "】"
	if res.ConnectSuccess {
		el.Success(fmt.Sprintf("%s连接成功，耗时 %.3fs", ip, res.ConnectCostTime))
	} else {
		errMsg := "未知错误"
		if res.Error != nil && *res.Error != "" {
			errMsg = *res.Error
		}
		el.Error(fmt.Sprintf("%s连接失败：%s", ip, errMsg))
	}

	if res.ConnectSuccess {
		switch r.mode {
		case "upload":
			if res.FailedFiles == 0 {
				el.Success(fmt.Sprintf("%s上传完成：成功 %d/%d 个文件", ip, res.SuccessFiles, res.TotalFiles))
			} else {
				el.Warn(fmt.Sprintf("%s上传完成：成功 %d/%d 个文件（有失败项）", ip, res.SuccessFiles, res.TotalFiles))
			}
		case "download":
			if res.FailedFiles == 0 {
				el.Success(fmt.Sprintf("%s下载完成：成功 %d/%d 个文件", ip, res.SuccessFiles, res.TotalFiles))
			} else {
				el.Warn(fmt.Sprintf("%s下载完成：成功 %d/%d 个文件（有失败项）", ip, res.SuccessFiles, res.TotalFiles))
			}
		default:
			if res.ExitCode != nil && *res.ExitCode == 0 {
				el.Success(fmt.Sprintf("%s命令执行成功，退出码 0，耗时 %.3fs", ip, res.ExecCostTime))
			} else {
				code := "无"
				if res.ExitCode != nil {
					code = fmt.Sprintf("%d", *res.ExitCode)
				}
				el.Error(fmt.Sprintf("%s命令执行失败，退出码 %s，耗时 %.3fs", ip, code, res.ExecCostTime))
			}
		}
	}
	el.Info(fmt.Sprintf("%s分类: %s", ip, category))
}

// deref 解引用可选字符串指针（nil 给空串）。
func deref(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
