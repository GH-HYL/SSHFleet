// 运行期呈现器（单节点结果的三个去向：终端明细 / output.txt / 执行期日志）。
//
// 为什么住这里：一条结果的「分类 → 三去向」是一件事，原先整段编排住在 main 的闭包里，
// 连同并发敏感的进度界面懒加载（互斥锁 + nil 判断）一起。那让 main 承担了本该属于
// 呈现层的职责，也让这段行为完全无法在测试里复现（只能真跑一次 SSH）。
//
// 边界：本模块只做「呈现」，不控制生命周期——main 仍独占主干与退出权，
// batch 的 Hooks 形状不变，只是回调体从 main 的闭包变成这里的三个方法。
//
// 进度界面从 bubbletea 取得（见 progress.go）。Program 的启动 / 收尾 / 退化都在这里：
// 首个进度事件才启动（保住「采集期提示先落终端」，用户 2026-09-14 裁定），
// Stop 时退出并等它收尾；输出不是终端（重定向 / 管道）时不启动，走直通模式。
package output

import (
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mattn/go-isatty"

	"sshfleet/internal/batch"
	"sshfleet/internal/log"
	"sshfleet/internal/result"
	"sshfleet/internal/ssh"
)

// Reporter 执行期呈现器：接住 batch 的三个事件（采集提示 / 进度 / 单节点结果），
// 把每一条结果送往它该去的地方。
//
// 接口（调用方需要知道的全部）：
//   - 用 NewReporter 构造，构造时一次性给出执行期日志、output.txt 句柄、分类依据、
//     模式与计时起点
//   - Notice / Progress / Result 三个方法与 batch.Hooks 的三个字段一一对应，可直接赋值
//   - 用完后调 Stop 收尾（进度界面落回界面下方），此后不应再收到事件
type Reporter struct {
	logger   *log.Logger
	outFile  io.Writer
	mode     string
	keyword  *result.Keywords
	total    int
	start    time.Time
	classify func(ssh.Result) string

	// out 进度界面与运行期提示的去向（默认 os.Stdout）。抽成字段是为了可测：
	// 测试里能把它换成临时文件，强制走一遍 bubbletea 那条路径。
	out *os.File

	mu   sync.Mutex
	prog *tea.Program
	// plain 直通模式：输出不是终端时不启动进度界面，明细与提示直接落 out。
	// 否则重定向到文件时会混进成千上万条光标控制序列（用户 2026-09-15 裁定）。
	plain bool
}

// NewReporter 构造呈现器。
//   - execLog: 执行期日志（已轮转至归档目录；nil 表示不写）
//   - outputFile: output.txt 句柄（nil 表示不写）
//   - total: 节点总数（进度界面的分母，懒创建时用）
//   - start: 计时起点（主干传 execStart）。进度界面的耗时由它算起，与统计块的
//     总耗时同源——进度条不再比总耗时少一截（用户 2026-09-15 裁定对齐口径）
//
// 分类依据（模式 + 错误关键词）在这里收下，调用方不必再自己拼分类函数。
func NewReporter(execLog *log.Logger, outputFile io.Writer, mode string, total int, kw *result.Keywords, start time.Time) *Reporter {
	r := &Reporter{
		logger:  execLog,
		outFile: outputFile,
		mode:    mode,
		keyword: kw,
		total:   total,
		start:   start,
		out:     os.Stdout,
		plain:   !isTerminal(os.Stdout),
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

// Progress 进度事件：首个事件才启动进度界面（采集期提示已先落终端）。
func (r *Reporter) Progress(s batch.Snapshot) {
	prog := r.ensureProgram()
	if prog == nil {
		return // 直通模式：不打进度条，明细照常
	}
	prog.Send(s)
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

// Stop 收尾：进度界面退出、光标落回界面下方，后续输出不再覆盖它。幂等。
func (r *Reporter) Stop() {
	r.mu.Lock()
	prog := r.prog
	r.prog = nil
	r.mu.Unlock()
	if prog == nil {
		return
	}
	prog.Quit()
	prog.Wait() // 等渲染收尾，确保统计块从进度块下方开始
}

// ensureProgram 懒创建进度界面：首个进度事件才启动，好让采集期提示先落终端
// （用户 2026-09-14 裁定）。直通模式（输出不是终端）下不建，返回 nil。
// 启动失败也退回直通模式——界面问题绝不该拖累执行。
func (r *Reporter) ensureProgram() *tea.Program {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.prog != nil || r.plain {
		return r.prog
	}

	p := tea.NewProgram(newProgressModel(r.mode, r.total, r.start),
		tea.WithOutput(r.out),
		tea.WithInput(nil),         // 不接输入：中断仍由主干的信号处理负责
		tea.WithoutSignalHandler(), // 不抢 SIGINT，免得与 signal.NotifyContext 打架
	)
	// Run 是阻塞的（bubbletea 里 Start 就是 Run 的别名），必须放进自己的 goroutine；
	// Send 会在事件循环就绪前自动等待，所以这里不需要额外的就绪同步。
	// 若事件循环异常退出，后续 Send 退化为 no-op——界面不显示，执行照常。
	go func() {
		if _, err := p.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "%s[警告]%s 进度界面异常退出：%v\n",
				ansiYellow, ansiReset, err)
		}
	}()
	r.prog = p
	return p
}

// printAbove 在进度界面上方打印；界面未启动（或直通模式）时直接落 out。
func (r *Reporter) printAbove(text string) {
	r.mu.Lock()
	prog := r.prog
	r.mu.Unlock()
	if prog != nil {
		prog.Println(text)
		return
	}
	fmt.Fprintln(r.out, text)
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

// isTerminal 输出是否连在终端上（含 Windows 的 cygwin/mintty 管道）。
func isTerminal(f *os.File) bool {
	fd := f.Fd()
	return isatty.IsTerminal(fd) || isatty.IsCygwinTerminal(fd)
}

// deref 解引用可选字符串指针（nil 给空串）。
func deref(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
