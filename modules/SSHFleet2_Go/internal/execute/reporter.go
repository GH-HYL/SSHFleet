package execute

// 一条结果的三个去处：屏幕、`terminal-output.txt`、执行期日志。
//
// **前两处是同一段字**：给人看的，一行一台，句子由 `message` 造一次（`message.ResultLine`），
// 两处读同一个值——版面只有一版，屏幕上与归档里对不上就没人能对账。
//
// **日志另走一条**：它是记录，不是给人看的版面（开发者 2026-11-02：「终端输出和 output 是
// 给人看的，当然要一致，log 是记录的」）。一台在日志里是五级明细（见 `log_node.go`），
// 维护者要按 IP 过滤、要分清「哪一步、什么现象」。

import (
	"fmt"
	"os"
	"sync"

	"sshfleet2/internal/console"
	"sshfleet2/internal/contract"
	"sshfleet2/internal/log"
	"sshfleet2/internal/message"
)

// reporter 执行期的去向集合。
//
// 那把锁罩的是「写这三处」那一段：worker 是并发跑完的，两个节点同时写同一个文件与同一个
// 日志句柄，不罩就会把两段字搅在一起（老版在呈现层也有一把同样的锁）。
type reporter struct {
	mu       sync.Mutex
	file     *os.File
	lg       *log.Logger
	mode     contract.Mode
	termPath string // 日志里「完整内容见 …」那一句要它；本包不拼路径，只转手
}

// newReporter 开两个落点：终端输出文件与执行期日志。
//
// 两个路径都由归档包发、塔尖转手；本包不拼路径，也不自己起目录——目录归归档包建，
// 这里建不出来就是上游没给对，不默默补一个（与 04 号单 `log.InitExec` 同一口径）。
func newReporter(termPath, execLogPath string, mode contract.Mode) (*reporter, error) {
	f, err := os.Create(termPath)
	if err != nil {
		return nil, fmt.Errorf("建终端输出文件失败（%s）：%w", termPath, err)
	}
	lg, err := log.InitExec(execLogPath)
	if err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("建执行期日志失败（%s）：%w", execLogPath, err)
	}
	return &reporter{file: f, lg: lg, mode: mode, termPath: termPath}, nil
}

// result 一条结果：屏幕与终端输出文件拿到同一段字，执行期日志写节点明细。
//
// 来一条写一条，不攒：防的是中途被强杀时盘上什么都没有（那是本单的目的，不是优化）。
func (r *reporter) result(res contract.NodeResult, user string) {
	// 拼字在锁外面：一条结果只拼这一次，屏幕与文件读的是同一个值。
	line := message.ResultLine(res, r.mode)

	r.mu.Lock()
	defer r.mu.Unlock()

	console.Print(line)
	if r.file != nil {
		_, _ = r.file.WriteString(line.Text())
	}
	logNode(r.lg, res, r.mode, r.termPath, user)
}

// close 收掉两个句柄；幂等。
func (r *reporter) close() {
	if r == nil {
		return
	}
	if r.file != nil {
		_ = r.file.Close()
	}
	if r.lg != nil {
		_ = r.lg.Close()
	}
}
