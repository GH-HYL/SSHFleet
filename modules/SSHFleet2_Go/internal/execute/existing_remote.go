package execute

// 上传的远端重名：开传之前整批问一次。
//
// 逐文件 `Stat` 是每个文件一次往返，几千个文件就是几千次；一条远端循环把整批落点一起问掉，
// 只付一次。代价是问的时候还没开始写——重名在写入之前就发现，目标机上不会有本次的残留。
//
// 谓词是 `[ -e "$p" ] || [ -L "$p" ]`：`-e` 跟随软链接，断链与自指环在它眼里都是「不存在」，
// 可这两个名字确实被占着，紧接着的 `Create` 会顺着链接写到别处；`-L` 不跟随、只看名字。
// 两个并起来才是「这个名字被占着」。
//
// 壳不认这条（csh 一类）时退回逐文件 `Stat`：慢，也看不见断链——与换这条之前同一口径，
// 但不让判重整块失效。

import (
	"strings"
)

// dedupeLoop 一条远端循环：名字被占着的那些按 NUL 打回来。
//
// `if` 包住 printf 是为了退出码：命中与否都交 0，只有命令本身没跑成才非 0。判重靠这一点
// 分开「没有重名」与「这条命令坏了」——实测裸写 `[ -e ] || [ -L ] && printf` 时，一条都没
// 命中会交 1，与命令坏了撞在同一个值上。
//
// 提权那一路必须用 `sh -c` 包起来：`sudo -n for …` 里 `for` 是 shell 关键字、不是命令，
// bash 当场报 `syntax error near unexpected token 'do'`（退出码 2，一个都没执行）。
func dedupeLoop(paths []string, useSudo bool) string {
	return loopOver(paths, `if [ -e "$p" ] || [ -L "$p" ]; then printf '%s\0' "$p"; fi`, useSudo)
}

// remoteExisting 问出这一批落点里已经被占着的那些，交回完整远端路径。
//
// 分批走 `cleanupBatchBytes`：那是远端命令行的长度上限，不是清理专用。
// 到了传输预算的边就停手，剩下的不猜——命中与否都还没问完，不能当作「没有重名」。
func (c *client) remoteExisting(clock *transferClock, sftpClient *sftpConn, paths []string, useSudo bool) ([]string, armEnd) {
	var hit []string
	for _, batch := range batchPaths(paths, cleanupBatchBytes) {
		out, err, end := c.execArmed(clock, dedupeLoop(batch, useSudo), true)
		if !end.ran() {
			return hit, end
		}
		if err != nil {
			// 这条命令没跑成：退回逐文件问，不让判重整块失效。
			return existingByStat(clock, sftpClient, paths)
		}
		hit = append(hit, splitRecords(out)...)
	}
	return hit, armRan
}

// existingByStat 退路：一条一条问（换这条之前的形态）。
func existingByStat(clock *transferClock, sftpClient *sftpConn, paths []string) ([]string, armEnd) {
	closeSFTP := func() { _ = sftpClient.Close() }
	var hit []string
	for _, p := range paths {
		var statErr error
		end := clock.arm(closeSFTP, func() { _, statErr = sftpClient.Stat(p) })
		if !end.ran() {
			return hit, end
		}
		if statErr == nil {
			hit = append(hit, p)
		}
	}
	return hit, armRan
}

// remoteNames 把完整远端路径还原成清单里的相对名：报出来的名字要是使用者认得的那份。
func remoteNames(hit []string, remotePath string) string {
	prefix := strings.TrimSuffix(remotePath, "/") + "/"
	names := make([]string, 0, len(hit))
	for _, p := range hit {
		names = append(names, strings.TrimPrefix(p, prefix))
	}
	return strings.Join(names, ", ")
}
