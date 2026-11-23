package execute

// 收尾：超时之后删掉本次写过的那些路径（ADR-0065）。
//
// 形态定死：在同一条 SSH 连接上另开一条**命令会话**，跑一条远端循环删完——不重拨、不重新
// 认证（第二次握手会重写服务端提示，而那一格参与分类），也不逐条开 SFTP 请求删（一个文件
// 一次往返，几千个文件会把收尾那份预算吃光）。
//
// 这一趟不看 ctx：它启动的那一刻 ctx 必然是取消状态（超时或中断），从这里派生就是过期的。
// 它只受自己那份预算约束——`timeout_connect`，到点即停，剩下的照实报。

import (
	"bytes"
	"time"
)

// cleanupBatchBytes 一条命令里路径总长的上限：远端命令行长度有限，几千个长路径装不下。
// 按长度分批，但**上限只有一份**——分批不重发（收尾的时间不随批数翻倍）。
const cleanupBatchBytes = 32 * 1024

// cleanupWritten 删掉本次写过的路径，返回删掉的个数、还留着的路径、以及那条命令有没有发出去。
//
// 「没来得及」与「没删干净」的分界：会话开不起来、或开起来就已经耗完预算，算没来得及
// （attempted=false，留下的按全部算）；发出去了、只是有路径没删掉，算没删干净，照打回来的报。
func (c *client) cleanupWritten(clock *transferClock, paths []string, useSudo bool) (removed int, left []string, attempted bool) {
	paths = dedupePaths(paths)
	if len(paths) == 0 {
		return 0, nil, true
	}

	batches := batchPaths(paths, cleanupBatchBytes)
	for i, batch := range batches {
		out, ok := c.runCleanupBatch(clock, cleanupLoop(batch, useSudo))
		if !ok {
			// 这一批没跑完：它自己与后面几批都算还留着——宁可报全，不许漏报。
			if !attempted {
				return removed, paths, false
			}
			var rest []string
			for _, b := range batches[i:] {
				rest = append(rest, b...)
			}
			return removed, dedupePaths(left, rest), true
		}
		attempted = true
		failed := splitRecords(out)
		removed += len(batch) - len(failed)
		left = append(left, failed...)
	}
	return removed, left, attempted
}

// runCleanupBatch 在收尾预算内跑一条删除命令；预算不够、命令没跑完都交回 false。
func (c *client) runCleanupBatch(clock *transferClock, command string) (string, bool) {
	// 预算已经用完就不再开会话；0 表示不设上限，交给 withinFinish 直接跑（与 arm 同口径）。
	if clock.finish > 0 && clock.finishRemaining() <= 0 {
		return "", false
	}
	sess, err := c.conn.NewSession()
	if err != nil {
		return "", false
	}
	defer func() { _ = sess.Close() }()

	var stderr bytes.Buffer
	sess.Stderr = &stderr

	var out []byte
	// 退出码与非 0 的错误不看：删不掉的路径由命令自己打回 stdout，照实的就在 out 里。
	ok := clock.withinFinish(func() { _ = sess.Close() }, func() { out, _ = sess.Output(command) })
	if !ok {
		return "", false
	}
	return string(out), true
}

// withinFinish 在收尾预算内等一次发起。
//
// 有的发起没有可关的通道（`conn.NewSession`），到点只能放弃；调用方据此记「没来得及」。
//
// **与 `arm` 的差别就在这一步**：`arm` 叫停之后还要等那次调用退出来，因为写还没退就去删，
// 删的是正在写的那个（ADR-0065 决定 5）。这里不等——清理命令等不等，后果只是「打回来的
// 清单短一点」，而等它会把下一批的预算吃光，等于让后面几批一律报「没来得及」。所以宁可
// 到点就交卷、把没删掉的按残留报全。
//
// 0 表示不设上限：配置层不许 0（`execution.timeout_connect` 至少 1 秒），这条路只留给直接
// 拼纸的调用方——与 `arm` 里 `transfer <= 0` 那条同口径，不能因为预算缺席就把清理整段跳过。
func (k *transferClock) withinFinish(stop func(), run func()) bool {
	if k.finish <= 0 {
		run()
		return true
	}
	wait := k.finishRemaining()
	if wait <= 0 {
		return false
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		run()
	}()
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case <-done:
		return true
	case <-timer.C:
		if stop != nil {
			stop()
		}
		return false
	}
}

// cleanupLoop 一条远端循环：删得掉的删掉，删不掉的把路径按 NUL 打回来。
//
// 不扫目录、不用通配符：清单里的路径都是本次自己建的（ADR-0022 决定 2）。
func cleanupLoop(paths []string, useSudo bool) string {
	return loopOver(paths, `rm -f -- "$p" || printf '%s\0' "$p"`, useSudo)
}

// batchPaths 按总长度把路径分批发出去：每批一条命令，上限只有一份。
func batchPaths(paths []string, limit int) [][]string {
	var (
		batches [][]string
		batch   []string
		size    int
	)
	for _, p := range paths {
		if len(batch) > 0 && size+len(p)+1 > limit {
			batches = append(batches, batch)
			batch, size = nil, 0
		}
		batch = append(batch, p)
		size += len(p) + 1
	}
	if len(batch) > 0 {
		batches = append(batches, batch)
	}
	return batches
}
