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
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mattn/go-isatty"

	"sshfleet/internal/batch"
	"sshfleet/internal/common"
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
	line := ResultLine(res, r.mode, category)

	// output.txt 只落文件：终端明细改经 printAbove（进度界面上方），
	// 与进度条各占一块区域、互不覆盖。
	if r.outFile != nil {
		_, _ = fmt.Fprintln(r.outFile, line)
	}

	if r.mode == "execute" {
		r.printAbove(line)
	}
	r.logNode(res, category)
}

// Category 单条结果的分类名（xlsx 导出等收尾环节按同一口径取用，
// 避免调用方自己再拼一份分类适配）。
func (r *Reporter) Category(res ssh.Result) string { return r.classify(res) }

// Stop 收尾：先让进度界面渲染一帧「终帧」（各条按目标值定格），再退出、
// 光标落回界面下方，后续输出不再覆盖它。幂等。
func (r *Reporter) Stop() {
	r.mu.Lock()
	prog := r.prog
	r.prog = nil
	r.mu.Unlock()
	if prog == nil {
		return
	}
	// 必须经由终帧消息退出，不能直接 Quit：进度条读的是弹簧动画的当前值，
	// 而 Stop 紧跟着最后一个节点完成到来，动画往往还停在半路——节点进度已经
	// 72/72，条却停在 97%（用户 2026-09-15 实测）。终帧由事件循环渲染、
	// 渲染完才执行退出，所以这一帧一定会出现在屏幕上。
	prog.Send(finalMsg{})
	prog.Wait()
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
			msg := fmt.Sprintf("进度界面异常退出：%v", err)
			fmt.Fprintf(os.Stderr, "%s[警告]%s %s\n", ansiYellow, ansiReset, msg)
			if r.logger != nil {
				r.logger.Warn(msg)
			}
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

// 执行期日志里单个节点的「输出明细」上限：超出只报数，完整内容在 output.txt 与 output.xlsx。
//
// 为什么要把输出写进日志：失败节点的输出就是排障第一现场——密码过期的
// 「Password change required but no TTY available」、nologin 的「此帐户目前不可用。」、
// 逐文件失败的原因，全都只在那里。此前这些内容只进终端与 output.txt，日志里一片空白，
// 事后翻日志只剩一句「命令执行失败，退出码 1」，看不出为什么。
//
// 为什么要封顶：`cat 大文件` 这类命令能产出上万行，日志不能无上限地长。
const (
	maxOutputLines   = 50
	maxOutputLineLen = 500
)

// logNode 把单节点结果按运行事件写入执行期日志（对位旧引擎的
// 「SSH连接成功/失败 → 执行结束/节点完成 → 分类」三级记录，全部带时间戳与级别）。
//
// 五级顺序：连接结果 → 执行结果 → 失败原因原文 → 输出明细 → 分类。
// 「失败原因」与「输出明细」是本次补齐的两级：结果字段里一直有完整报文
//（Error 是原因原文，Output 是输出/明细），但此前只有连接失败那一条被写进日志。
func (r *Reporter) logNode(res ssh.Result, category string) {
	el := r.logger
	if el == nil {
		return
	}
	ip := "【" + res.IP + "】"

	// 成功节点：合成一行就走。
	// 目标机常是 1000+ 台，每个节点铺开三行会把日志刷满；正常路径只需要
	// 「哪台跑了、多快」。失败节点才展开细节（下方逐条写、不合并）。
	if !isFailedResult(res) {
		el.Success(ip + "成功：" + r.successDetail(res))
		return
	}

	// 一级：连接。失败时把报错原文（含服务端提示）逐行写下——这是「连不上」
	// 这类结果唯一的原因来源，日志里没有它就只剩一个「连接失败」的空壳。
	if res.ConnectSuccess {
		el.Success(fmt.Sprintf("%s连接成功%s，耗时 %.3fs",
			ip, joinNotes(userNote(res.User), authNote(res.AuthMethod)), res.ConnectCostTime))
	} else {
		logMultiline(el.Error, ip, "连接失败"+joinNotes(userNote(res.User))+"：", errText(res, "未知错误"))
	}

	// 二级：执行结果（命令看退出码，传输看成功/失败文件数与字节数）
	if res.ConnectSuccess {
		switch r.mode {
		case "upload", "download":
			action := "上传"
			if r.mode == "download" {
				action = "下载"
			}
			note := ""
			var write func(...any) = el.Success
			if res.FailedFiles > 0 {
				note, write = fmt.Sprintf("（有失败项 %d 个）", res.FailedFiles), el.Warn
			}
			write(fmt.Sprintf("%s%s完成：成功 %d/%d 个文件%s，共 %s，耗时 %.3fs",
				ip, action, res.SuccessFiles, res.TotalFiles, note, common.FormatBytes(res.TotalBytes), res.ExecCostTime))
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

	// 三级：失败原因原文。连接失败已在上面写过；这里补「连上了却没跑成」的原因
	//（创建会话失败 / 命令执行超时 / 远程路径不存在 / 逐文件失败……）。
	if res.ConnectSuccess && res.Error != nil && *res.Error != "" {
		logMultiline(el.Error, ip, "错误详情：", *res.Error)
	}

	// 四级：输出明细。只在失败节点记——成功节点的输出可能是整份文件内容。
	// 与「错误详情」逐字相同的输出不再重复写一遍（传输模式下只有一个文件失败就是这种）。
	if isFailedResult(res) && res.Output != "" && !sameAsError(res) {
		logOutputBlock(el.Warn, ip, res.Output)
	}

	// 五级：分类
	el.Info(fmt.Sprintf("%s分类: %s", ip, category))
}

// successDetail 成功节点那行的可变部分（按模式给「跑了什么、多快」）。
func (r *Reporter) successDetail(res ssh.Result) string {
	conn := fmt.Sprintf("连接 %.3fs，", res.ConnectCostTime)
	switch r.mode {
	case "upload":
		return fmt.Sprintf("%s上传 %d/%d 个文件（%s），耗时 %.3fs",
			conn, res.SuccessFiles, res.TotalFiles, common.FormatBytes(res.TotalBytes), res.ExecCostTime)
	case "download":
		return fmt.Sprintf("%s下载 %d/%d 个文件（%s），耗时 %.3fs",
			conn, res.SuccessFiles, res.TotalFiles, common.FormatBytes(res.TotalBytes), res.ExecCostTime)
	default:
		return fmt.Sprintf("%s执行 %.3fs", conn, res.ExecCostTime)
	}
}

// errText 取结果里的报错原文（nil 或空时给 fallback）。
func errText(res ssh.Result, fallback string) string {
	if res.Error != nil && *res.Error != "" {
		return *res.Error
	}
	return fallback
}

// userNote / authNote 日志里的上下文片段（值为空时给空串，不占位）。
func userNote(user string) string {
	if user == "" {
		return ""
	}
	return "用户 " + user
}

func authNote(authMethod string) string {
	if authMethod == "" {
		return ""
	}
	return "登录方式 " + authMethod
}

// joinNotes 把若干个可选片段拼成「，a，b」；全空时给空串。
func joinNotes(parts ...string) string {
	kept := make([]string, 0, len(parts))
	for _, p := range parts {
		if p != "" {
			kept = append(kept, p)
		}
	}
	if len(kept) == 0 {
		return ""
	}
	return "，" + strings.Join(kept, "，")
}

// isFailedResult 该结果是否算失败（决定要不要把输出明细写进日志）。
// 连接失败、有失败文件、退出码非 0 或缺席、带报错原文，任一成立即为失败。
func isFailedResult(res ssh.Result) bool {
	if !res.ConnectSuccess || res.FailedFiles > 0 {
		return true
	}
	if res.ExitCode == nil || *res.ExitCode != 0 {
		return true
	}
	return res.Error != nil && *res.Error != ""
}

// sameAsError 输出明细与报错原文是否逐字相同（相同则不必再写一遍明细）。
func sameAsError(res ssh.Result) bool {
	if res.Error == nil || res.Output == "" {
		return false
	}
	return strings.Join(splitLines(res.Output), "\n") == strings.Join(splitLines(*res.Error), "\n")
}

// logMultiline 把可能含多行的报错原文按行写入日志：首行接在 head 后面，
// 其余行缩进对齐，每行都带节点前缀——既能按 IP 过滤，也保证每行时间戳齐全。
func logMultiline(fn func(...any), ip, head, text string) {
	lines := splitLines(text)
	if len(lines) == 0 {
		fn(ip + head)
		return
	}
	for i, ln := range lines {
		if i == 0 {
			fn(ip + head + truncateLine(ln))
			continue
		}
		fn(ip + "  " + truncateLine(ln))
	}
}

// logOutputBlock 把失败节点的输出明细按行写入日志（带行数抬头与上限）。
func logOutputBlock(fn func(...any), ip, output string) {
	lines := splitLines(output)
	if len(lines) == 0 {
		return
	}
	fn(fmt.Sprintf("%s输出明细（%d 行）：", ip, len(lines)))

	shown, rest := lines, 0
	if len(lines) > maxOutputLines {
		shown, rest = lines[:maxOutputLines], len(lines)-maxOutputLines
	}
	for _, ln := range shown {
		fn(ip + "  " + truncateLine(ln))
	}
	if rest > 0 {
		fn(fmt.Sprintf("%s  ……其余 %d 行省略（完整内容见 output.txt / output.xlsx）", ip, rest))
	}
}

// splitLines 按行拆分并去掉空行：日志每行都已带前缀与时间戳，空行只会把版面拉稀。
// 传输明细首行的 `total_files=…` 统计头也在此剔除——执行结果行已经报过同一组数。
func splitLines(text string) []string {
	raw := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	out := make([]string, 0, len(raw))
	for _, ln := range raw {
		ln = strings.TrimRight(ln, " \t\r")
		if strings.TrimSpace(ln) == "" || strings.HasPrefix(ln, "total_files=") {
			continue
		}
		out = append(out, ln)
	}
	return out
}

// truncateLine 单行超长时截断（超长行多为 JSON / base64 这类内容）。
func truncateLine(s string) string {
	if utf8.RuneCountInString(s) <= maxOutputLineLen {
		return s
	}
	return string([]rune(s)[:maxOutputLineLen]) + "…"
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
