package execute

// 一条结果的三个去处：屏幕、`terminal-output.txt`、执行期日志。
//
// 三处拿的是**同一段字**：句子由 `message` 造一次（`message.ResultLine`），本文件只负责
// 把它交给三个落点。三处各拼一遍就是三个会各自漂的来源——屏幕上与归档里对不上，事后没法对账。
//
// 版式、字段、分隔线全在 `message` 里，这里一个字都不拼。

import (
	"fmt"
	"os"
	"strings"
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
	mu   sync.Mutex
	file *os.File
	lg   *log.Logger
	mode contract.Mode
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
	return &reporter{file: f, lg: lg, mode: mode}, nil
}

// result 一条结果：屏幕上一段、终端输出文件一段、执行期日志一条。
//
// 来一条写一条，不攒：防的是中途被强杀时盘上什么都没有（那是本单的目的，不是优化）。
func (r *reporter) result(res contract.NodeResult) {
	// 拼字在锁外面：一条结果只拼这一次，三个去处读的是同一个值。
	line := message.ResultLine(res, r.mode)
	text := line.Text()

	r.mu.Lock()
	defer r.mu.Unlock()

	console.Print(line)
	if r.file != nil {
		_, _ = r.file.WriteString(text)
	}
	if r.lg != nil {
		// 日志每一行自带换行（前缀 + 正文 + \n），正文末尾那个就不必再带——带了会在日志里
		// 多一条空行。
		r.lg.Info(strings.TrimSuffix(text, "\n"))
	}
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
