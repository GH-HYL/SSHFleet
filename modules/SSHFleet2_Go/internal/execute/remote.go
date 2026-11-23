package execute

// 辅助命令：工具在远端额外跑的那些（`test -e`、`find`、判重与清理循环）。
//
// 它们不是「目的命令」：退出码进 `NodeResult.Steps`，不顶成结果退出码——否则判定会给出
// 「执行失败(退出码N)」，把远端说的原因整个盖掉。
//
// 这一文件放两处以上在用的三样：会话怎么开、回话怎么切、循环怎么包。

import (
	"strings"

	"sshfleet2/internal/criterion"
)

// recordSep 是一条命令用 NUL 收尾记录时的那一格：枚举、判重、清理都按它切。
const recordSep = "\x00"

// splitRecords 按 NUL 切一条命令的 stdout，空段丢掉。
func splitRecords(out string) []string {
	var recs []string
	for _, rec := range strings.Split(out, recordSep) {
		if rec != "" {
			recs = append(recs, rec)
		}
	}
	return recs
}

// execArmed 跑一条辅助命令，按传输预算武装：到点就关掉这条会话、等它退出来。
//
// capture 为真时只收 stdout：枚举与判重的记录按 NUL 切，远端在 stdout 上多说一句会被当成
// 一条坏记录。
func (c *client) execArmed(clock *transferClock, command string, capture bool) (string, error, armEnd) {
	session, err := c.conn.NewSession()
	if err != nil {
		return "", err, armRan
	}
	defer func() { _ = session.Close() }()

	var (
		out    []byte
		runErr error
	)
	end := clock.arm(func() { _ = session.Close() }, func() {
		if capture {
			out, runErr = session.Output(command)
			return
		}
		runErr = session.Run(command)
	})
	return string(out), runErr, end
}

// loopOver 一条「逐个路径跑一段」的远端循环：路径逐个转义、包进单引号，循环体由调用方给。
//
// 判重与清理只差循环体，头尾、引号与提权那层包装都只此一处。
func loopOver(paths []string, body string, useSudo bool) string {
	var b strings.Builder
	b.WriteString("for p in")
	for _, p := range paths {
		b.WriteString(" " + criterion.SingleQuoted(p))
	}
	b.WriteString("; do " + body + "; done")
	if useSudo {
		return sudoShell(b.String())
	}
	return b.String()
}

// sudoShell 把一条要用 root 跑的 shell 命令包起来。
//
// 不能写成 `sudo -n <命令>`：`for` 是 shell 关键字不是可执行文件，bash 当场报
// `syntax error near unexpected token 'do'`、退出码 2，一条都没执行——而收尾那一路不看
// 退出码，于是会把「什么都没删」报成「删干净了」。包一层 `sh -c` 才是真在 root 下跑这个循环。
func sudoShell(command string) string {
	return "sudo -n sh -c " + criterion.SingleQuoted(command)
}
