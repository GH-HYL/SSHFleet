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
	"sshfleet/internal/cli"
	"sshfleet/internal/common"
	"sshfleet/internal/config"
	"sshfleet/internal/log"
	"sshfleet/internal/ssh"
	"sshfleet/internal/verdict"
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
	logger  *log.Logger
	outFile io.Writer
	mode    cli.Mode
	// answers 代填表（-a 的解析结果，顺序即文本里的行序）。只为一件事存在：
	// 把结果里「未送出的代填」序号还原成触发词原文（D35）。序号与用户表同源，
	// 呈现层查一次即可，不必在每个结果里复制一份事实。
	answers []ssh.Answer
	total   int
	start   time.Time

	// out 进度界面与运行期提示的去向（默认 os.Stdout）。抽成字段是为了可测：
	// 测试里能把它换成临时文件，强制走一遍 bubbletea 那条路径。
	out *os.File

	mu   sync.Mutex
	prog *tea.Program
	// stats 进度链路计时（排障用）。只记数、不改变行为：见 progressdebug.go。
	stats *progressStats
	// plain 直通模式：输出不是终端时不启动进度界面，明细与提示直接落 out。
	// 否则重定向到文件时会混进成千上万条光标控制序列（用户 2026-09-15 裁定）。
	plain bool

	// quiet 非交互模式（--yes）：运行期提示只进日志、不上屏（L61 的静默总闸门）。
	// 报错与结果明细不受影响——挡的只有"教学类"那一路。
	quiet bool
}

// NewReporter 构造呈现器。
//   - execLog: 执行期日志（已轮转至归档目录；nil 表示不写）
//   - outputFile: output.txt 句柄（nil 表示不写）
//   - mode: 执行模式（决定展示用的分类名与日志措辞）
//   - answers: 代填表（-a 的解析结果；没有代填时给 nil）——「未送出的代填」要靠它
//     把序号还原成触发词原文
//   - total: 节点总数（进度界面的分母，懒创建时用）
//   - start: 计时起点（主干传 execStart）。进度界面的耗时由它算起，与统计块的
//     总耗时同源——进度条不再比总耗时少一截（用户 2026-09-15 裁定对齐口径）
//   - quiet: 非交互模式（--yes）。运行期提示不上屏，只进日志（L61 静默闸门）
//
// 成败与分类不在这里定——结果到手时判定已在 batch 的 worker 协程里做完，呈现层只读结论。
func NewReporter(execLog *log.Logger, outputFile io.Writer, mode cli.Mode, answers []ssh.Answer, total int, start time.Time, quiet bool) *Reporter {
	return &Reporter{
		logger:  execLog,
		outFile: outputFile,
		mode:    mode,
		answers: answers,
		total:   total,
		start:   start,
		out:     os.Stdout,
		stats:   newProgressStats(execLog),
		plain:   !isTerminal(os.Stdout),
		quiet:   quiet,
	}
}

// Notice 采集期提示（目前仅上传源中被过滤的链接）：终端 + 执行期日志。
func (r *Reporter) Notice(msg string) {
	// 非交互模式：提示只留日志痕迹，不上屏（L61 的静默总闸门）
	if !r.quiet {
		r.printAbove(msg)
	}
	if r.logger != nil {
		r.logger.Info(msg)
	}
}

// Progress 进度事件：首个事件才启动进度界面（采集期提示已先落终端）。
//
// Send 是阻塞发（消息队列无缓冲），返回时只代表事件循环把这条消息收下了。这中间的等待
// 正是「进度为什么不动」的直接读数，故计时后写一条 DEBUG（见 progressdebug.go）。
func (r *Reporter) Progress(s batch.Snapshot) {
	prog := r.ensureProgram()
	if prog == nil {
		return // 直通模式：不打进度条，明细照常
	}
	sent := time.Now()
	prog.Send(s)
	r.stats.recordSnapSend(time.Since(sent), s)
}

// Result 单节点结果：写 output.txt（各模式）+ 命令/脚本与改密打终端明细 + 写执行期日志。
// 成败与分类读判定写好的结论；展示分类（成功行的中文名）按模式合成。
func (r *Reporter) Result(res ssh.Result) {
	// 展示分类每结果只算一次：明细行与日志行读同一个值。
	category := displayCategory(res, r.mode)
	line := ResultLine(res, r.mode, r.answers, category)

	// output.txt 只落文件：终端明细改经 printAbove（进度界面上方），
	// 与进度条各占一块区域、互不覆盖。
	if r.outFile != nil {
		_, _ = fmt.Fprintln(r.outFile, line)
	}

	// 终端明细：非传输类（命令 / 脚本 / 改密）上屏；传输类（上传 / 下载）不上屏——
	// 那两条路有进度界面负责呈现明细。
	if !r.mode.Info().Transfer {
		r.printAbove(line)
	}
	r.logNode(res, category)
}

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
	// 界面收尾之后写小结：换环境对比时先看这一行（投递等待与渲染耗时都在上面）。
	r.stats.logSummary()
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

	p := tea.NewProgram(newProgressModel(r.mode, r.total, r.start, r.stats),
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
// Println 与进度快照走同一条无缓冲队列，等待时长同样记一条 DEBUG。
func (r *Reporter) printAbove(text string) {
	r.mu.Lock()
	prog := r.prog
	r.mu.Unlock()
	if prog != nil {
		sent := time.Now()
		prog.Println(text)
		r.stats.recordLineSend(time.Since(sent))
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
// （Error 是原因原文，Output 是输出/明细），但此前只有连接失败那一条被写进日志。
func (r *Reporter) logNode(res ssh.Result, category string) {
	el := r.logger
	if el == nil {
		return
	}
	ip := "【" + res.IP + "】"

	// 成功节点：合成一行就走。
	// 目标机常是 1000+ 台，每个节点铺开三行会把日志刷满；正常路径只需要
	// 「哪台跑了、多快」。失败节点才展开细节（下方逐条写、不合并）。
	if res.Verdict == verdict.Success {
		el.Success(ip + "成功：" + r.successDetail(res))
		return
	}

	// 一级：连接。失败时把报错原文（含服务端提示）逐行写下——这是「连不上」
	// 这类结果唯一的原因来源，日志里没有它就只剩一个「连接失败」的空壳。
	if res.ConnectSuccess {
		el.Success(fmt.Sprintf("%s连接成功%s，耗时 %.3fs",
			ip, joinNotes(userNote(res.User), authNote(res.AuthMethod)), res.ConnectCostTime))
	} else {
		logMultiline(el.Error, ip, "连接失败"+joinNotes(userNote(res.User))+"：", errText(res, r.answers, "未知错误"))
	}

	// 二级：执行结果（成败读判定结论；失败行把定论退出码带上——
	// 「退出码 0 却计入失败」是判定与退出码解绑的活证，照实显示）。
	//
	// 本级只写失败：成功在上面已提前 return，下面整块只有失败行走得到。
	// 各模式原先还各留了一个「成功」分支，那些行永远印不出来，而且 1d622fb
	// 那次「改从名字表取词」正是改在了不可达的地方——2026-10-07 一并清掉。
	if res.ConnectSuccess {
		switch r.mode {
		case cli.ModeUpload, cli.ModeDownload:
			action := r.mode.Info().ActionName
			note := ""
			var write func(...any) = el.Success
			if res.FailedFiles > 0 {
				note, write = fmt.Sprintf("（有失败项 %d 个）", res.FailedFiles), el.Warn
			}
			write(fmt.Sprintf("%s%s完成：成功 %d/%d 个文件%s，共 %s，耗时 %.3fs",
				ip, action, res.SuccessFiles, res.TotalFiles, note, common.FormatBytes(res.TotalBytes), res.ExecCostTime))
		case cli.ModePasswd:
			// 改密没有"目的命令"，不带退出码（定论退出码对改密恒为 nil）。
			// 词取自名字表（LogName =「改密」）。
			el.Error(fmt.Sprintf("%s%s失败，耗时 %.3fs", ip, r.mode.Info().LogName, res.ExecCostTime))
		default:
			// 命令 / 脚本共用这一路：前缀取自名字表（LogName + ActionName =「命令执行」/「脚本执行」）。
			// 原先写死「命令」，脚本模式跑完也报「命令」（2026-10-07 修）。
			//
			// ModeNone 到不了这里：cli/check.go 的五选一校验在「一个模式都没给」时就报错退出，
			// 而结果只由真跑过的机器产出。所以 label 为空、会拼出「失败，退出码 无」这种残缺
			// 句子的那一格**印不出来**，是**有意留着、不加守卫**的——2026-10-07 审计就此问过，
			// 结论：记录在此、保持现状。
			label := r.mode.Info().LogName + r.mode.Info().ActionName
			code := "无"
			if res.ExitCode != nil {
				code = fmt.Sprintf("%d", *res.ExitCode)
			}
			el.Error(fmt.Sprintf("%s%s失败，退出码 %s，耗时 %.3fs", ip, label, code, res.ExecCostTime))
		}
	}

	// 三级：失败原因原文。连接失败已在上面写过；这里补「连上了却没跑成」的原因
	//（创建会话失败 / 命令执行超时 / 远程路径不存在 / 逐文件失败 / 未送出的代填……）。
	// 文本 = 报错原文 + 服务端提示 + 未送出的代填合成（D34 / D35）。
	if res.ConnectSuccess && errText(res, r.answers, "") != "" {
		logMultiline(el.Error, ip, "错误详情：", errText(res, r.answers, ""))
	}

	// 四级：输出明细。只在失败节点记——成功节点的输出可能是整份文件内容。
	// 与「错误详情」逐字相同的输出不再重复写一遍（传输模式下只有一个文件失败就是这种）。
	if res.Output != "" && !sameAsError(res, r.answers) {
		logOutputBlock(el.Warn, ip, res.Output)
	}

	// 五级：分类
	el.Info(fmt.Sprintf("%s分类: %s", ip, category))
}

// successDetail 成功节点那行的可变部分（按模式给「跑了什么、多快」）。
func (r *Reporter) successDetail(res ssh.Result) string {
	conn := fmt.Sprintf("连接 %.3fs，", res.ConnectCostTime)
	switch r.mode {
	case cli.ModeUpload, cli.ModeDownload:
		return fmt.Sprintf("%s%s %d/%d 个文件（%s），耗时 %.3fs",
			conn, r.mode.Info().ActionName, res.SuccessFiles, res.TotalFiles,
			common.FormatBytes(res.TotalBytes), res.ExecCostTime)
	default: // 命令 / 脚本 / 改密：动作名 + 耗时
		return fmt.Sprintf("%s%s %.3fs", conn, r.mode.Info().ActionName, res.ExecCostTime)
	}
}

// errText 取结果里的失败详情文本：报错原文 → 服务端提示 → 未送出的代填 → 中间步骤，逐段换行合成。
//
// 前两段的合成沿用 ADR-0005 时代的顺序（原文在前、提示换行追加在后），D34 把提示
// 独立成字段后在这里合回一处；后两段是结构事实的细节（D35 / W10）。四段都空时给 fallback。
// 只在失败行调用——正常行不显示服务端提示（MOTD / 法务声明这类公告不该出现在成功的行里）。
func errText(res ssh.Result, answers []ssh.Answer, fallback string) string {
	parts := make([]string, 0, 4)
	for _, s := range []string{deref(res.Error), strings.TrimSpace(res.ServerBanner), missedNote(res, answers), stepsNote(res)} {
		if s != "" {
			parts = append(parts, s)
		}
	}
	if len(parts) == 0 {
		return fallback
	}
	return strings.Join(parts, "\n")
}

// missedNote 「未送出的代填」那行事实（D35）：把结果里的序号还原成触发词原文。
//
// 只陈述事实、不写结论词——「触发词未命中」四个字仍只由分类列表达。为什么不把它写进
// 执行侧：那正是上一轮治掉的病（结构事实降级成文案、下游再靠文本认回来）。
// 触发词取自 -a 的代填表（序号 1 起，与 Result.AnswersMissed 同源）；
// 序号越界时只报序号，不猜触发词。
func missedNote(res ssh.Result, answers []ssh.Answer) string {
	if len(res.AnswersMissed) == 0 {
		return ""
	}
	items := make([]string, 0, len(res.AnswersMissed))
	for _, n := range res.AnswersMissed {
		if n < 1 || n > len(answers) || len(answers[n-1].Triggers) == 0 {
			items = append(items, fmt.Sprintf("第 %d 条", n))
			continue
		}
		items = append(items, fmt.Sprintf("第 %d 条（触发词 %s）", n, quoteTriggers(answers[n-1].Triggers)))
	}
	return "未送出的代填：" + strings.Join(items, "、")
}

// quoteTriggers 一条代填的触发词原文：各加直引号、顿号相连（如 "a"、"b"）。
func quoteTriggers(triggers []string) string {
	quoted := make([]string, 0, len(triggers))
	for _, t := range triggers {
		quoted = append(quoted, `"`+t+`"`)
	}
	return strings.Join(quoted, "、")
}

// stepNames 中间步骤语义名 → 中文说明（含裸命令原文）。W10（2026-10-04）：
// StepResult.Name 已语义化，命令原文只在这里出现；未知名的兜底是原样显示语义名——
// 不猜、不编中文。
var stepNames = map[string]string{
	ssh.StepPrecheck:  "下载预检（test -e）",
	ssh.StepEnumerate: "远程文件枚举（find）",
	ssh.StepPasswd:    "改密收尾（:）",
}

// stepsNote 中间步骤那行事实（W10）：把 Steps 的语义名还原成中文说明、附各自的退出码。
//
// 与 missedNote 同一条规矩：只陈述事实、不写结论词——这一步是预检没过还是枚举出错，
// 由分类列与报错原文表达，这里只回答「工具在远端额外跑了什么、结果如何」。
// 退出码拿不到（超时 / 中断）就说拿不到，不推 0。
func stepsNote(res ssh.Result) string {
	if len(res.Steps) == 0 {
		return ""
	}
	items := make([]string, 0, len(res.Steps))
	for _, st := range res.Steps {
		label, ok := stepNames[st.Name]
		if !ok {
			label = st.Name
		}
		if st.ExitCode == nil {
			items = append(items, fmt.Sprintf("%s 未拿到退出码", label))
			continue
		}
		items = append(items, fmt.Sprintf("%s 退出码 %d", label, *st.ExitCode))
	}
	return "中间步骤：" + strings.Join(items, "、")
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

// sameAsError 输出明细与失败详情文本是否逐字相同（相同则不必再写一遍明细）。
func sameAsError(res ssh.Result, answers []ssh.Answer) bool {
	errStr := errText(res, answers, "")
	if errStr == "" || res.Output == "" {
		return false
	}
	return strings.Join(splitLines(res.Output), "\n") == strings.Join(splitLines(errStr), "\n")
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
		fn(fmt.Sprintf("%s  ……其余 %d 行省略（完整内容见 %s / %s）", ip, rest,
			config.BuiltinPaths.Output, config.BuiltinPaths.OutputXlsx))
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
