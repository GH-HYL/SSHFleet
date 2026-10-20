// 诊断器本体：把「一次运行里能判定根因的所有事实」写进日志。
//
// 设计约束（这一轮的前提：测试机会只有一次、由别人替我们跑）：
//   - 不依赖任何开关：编译进去就生效，他换 exe 跑一次就有；
//   - 信息进**同一个日志文件**（工具日志 history/SSHFleetTools.log），
//     不必让他去归档目录里翻多个文件；
//   - 每个节点写一段自洽的记录：远端原始字节 → 整备后字节 → 最终交给终端的字节，
//     三段并排，读的人一眼能看出「是采集丢了、还是终端吃了」；
//   - 结尾写一份汇总，只汇总计数与形态，不重复正文。
package diagnose

import (
	"fmt"
	"sync"
)

// 原始输出与最终文本各自最多记录多少字节（十六进制 + 转义两种形式各一份）。
// 够看清行尾与开头，又不至于把日志撑爆：几百台机器时每台 1~2KB。
const (
	rawDumpLimit   = 512
	rawEscapeLimit = 512
	// maxSamples 汇总里点名几个「远端回 CR」的节点。
	maxSamples = 5
)

// Recorder 诊断记录器。并发安全：结果由 worker 协程产出、呈现层单线程回放，
// 但原始输出是在采集侧（worker 协程）登记的，必须加锁。
type Recorder struct {
	lg Logger

	mu sync.Mutex
	// 每个节点只写一次的原始输出记录（键 = 序号）。
	rawWritten map[int]bool
	// 汇总用的计数：远端原始输出里带 CR 的节点、最终文本里带控制字符的节点。
	nodesWithCR   int
	nodesWithCtrl int
	nodesTotal    int
	// 前几个「远端回 CR」的节点，汇总里点名。
	samplesWithCR []string
}

// Logger 诊断器需要的最小日志能力（工具日志已满足；nil 安全）。
type Logger interface {
	Info(args ...any)
}

// New 建一个记录器；lg 为 nil 时所有方法都是空操作（测试与不诊断时不必判空）。
func New(lg Logger) *Recorder {
	return &Recorder{lg: lg, rawWritten: map[int]bool{}}
}

// Enabled 是否真的会记（没给日志时全空操作）。
func (r *Recorder) Enabled() bool { return r != nil && r.lg != nil }

// Environment 记一次运行的环境事实：版本、系统、终端、颜色能力、会话 tee 设定。
// 在开工之前调用一次即可——它是「终端会怎么解释这段字节」的前提条件。
func (r *Recorder) Environment(version, goos, goarch string, colorEnabled bool, termName, logPath string) {
	if !r.Enabled() {
		return
	}
	vt, vtNote := ConsoleVT()
	r.lg.Info("DIAG> ── 环境 ──────────────────────────────────────────")
	r.lg.Info("DIAG> 本文件就是取证日志，发回时请整个复制：" + logPath)
	r.lg.Info(fmt.Sprintf("DIAG> 版本：%s；平台：%s/%s；%s", version, goos, goarch, LocalEnv(colorEnabled, vt, termName)))
	r.lg.Info(fmt.Sprintf("DIAG> Windows控制台VT：%s", vtNote))
	r.lg.Info("DIAG> 会话不申请 tty（工具从不调用 RequestPty），远端按自身逻辑断行")
}

// RemoteRaw 记录某节点**整备之前**的远端原始输出。采集侧在归一前调用。
func (r *Recorder) RemoteRaw(seq int, ip, raw string) {
	if !r.Enabled() {
		return
	}
	r.mu.Lock()
	if r.rawWritten[seq] {
		r.mu.Unlock()
		return
	}
	r.rawWritten[seq] = true
	st := Stats(raw)
	if st.CR > 0 {
		r.nodesWithCR++
		if len(r.samplesWithCR) < maxSamples {
			r.samplesWithCR = append(r.samplesWithCR, fmt.Sprintf("【%s】CR=%d", ip, st.CR))
		}
	}
	r.mu.Unlock()

	r.lg.Info(fmt.Sprintf("DIAG> ── 节点【%s】远端原始输出（整备之前）──", ip))
	r.lg.Info(fmt.Sprintf("DIAG>   字节构成：%s", st))
	r.lg.Info(fmt.Sprintf("DIAG>   行尾形态：%s", Endings(raw)))
	r.lg.Info(fmt.Sprintf("DIAG>   转义形式：%s", quote(Escaped(raw, rawEscapeLimit))))
	if raw != "" {
		r.lg.Info("DIAG>   十六进制：")
		for _, line := range splitLines(HexDump(raw, rawDumpLimit)) {
			r.lg.Info("DIAG>   " + line)
		}
	}
}

// ResultDetail 记录某节点**最终交给终端**的那段文本。呈现层在打屏之前调用。
//
// 与 RemoteRaw 并排对照：原始有 CR、最终没有 → 采集侧归一生效了；
// 原始没有 CR、最终也没有、但屏幕上仍空白 → 问题不在字节，要另找。
func (r *Recorder) ResultDetail(seq int, ip, line string) {
	if !r.Enabled() {
		return
	}
	st := Stats(line)
	r.mu.Lock()
	r.nodesTotal++
	if st.CR > 0 || st.ESC > 0 || st.NUL > 0 || st.OtherCtrl > 0 {
		r.nodesWithCtrl++
	}
	r.mu.Unlock()

	r.lg.Info(fmt.Sprintf("DIAG> ── 节点【%s】最终交给终端的文本──", ip))
	r.lg.Info(fmt.Sprintf("DIAG>   字节构成：%s", st))
	r.lg.Info(fmt.Sprintf("DIAG>   行尾形态：%s", Endings(line)))
	r.lg.Info(fmt.Sprintf("DIAG>   转义形式：%s", quote(Escaped(line, rawEscapeLimit))))
}

// Summary 收尾汇总：一次运行的判据性计数。主干在结果处理完之后调用一次。
func (r *Recorder) Summary() {
	if !r.Enabled() {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lg.Info("DIAG> ══ 汇总 ══════════════════════════════════════════")
	r.lg.Info(fmt.Sprintf("DIAG> 收到结果 %d 条；远端原始输出带 CR 的节点 %d 个；最终文本仍带控制字符的节点 %d 个",
		r.nodesTotal, r.nodesWithCR, r.nodesWithCtrl))
	if len(r.samplesWithCR) > 0 {
		r.lg.Info("DIAG> 回 CR 的节点（前几个）：" + joinComma(r.samplesWithCR))
	}
	r.lg.Info("DIAG> 对照读法：原始有 CR 而最终没有 → 归一已生效；两者都没有却屏幕上空白 → 不在字节层")
}
