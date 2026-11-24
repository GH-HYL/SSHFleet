package execute

// 一台传输的账：四个数与明细行，只有一个出口。
//
// 上传与下载的早退路合起来十几条，以前各写一遍：四个数与明细行散在各处，新增一条路径忘写
// 一处，报告与日志里的数就印错——下载侧此前一条都没写，那两个数永远是 0。这里收成一处：
// 谁来记账就调那几个 add，收场只有一个 settle 把四个数与明细一起写进结果。
//
// 账不管「目标机上现在可能有什么」：写成功的、写到一半没删的清单留在上传自己手里，
// 下载用不上，塞进共用对象就是「传整个对象只用两个字段」的反面。

import "sshfleet2/internal/contract"

// transferAccount 一台传输的账。
type transferAccount struct {
	stat  contract.TransferStat
	lines []string
}

// newTransferAccount 开一本账：total 是这台打算传几个，lines 是随早退一起走的明细行。
func newTransferAccount(total int, lines []string) *transferAccount {
	return &transferAccount{stat: contract.TransferStat{TotalFiles: total}, lines: lines}
}

// count 补上总数：下载要枚举完才知道这台几个文件。
func (a *transferAccount) count(total int) { a.stat.TotalFiles = total }

// wrote 一个文件传成了：成功加一、字节累加。
func (a *transferAccount) wrote(n int64) {
	a.stat.SuccessFiles++
	a.stat.TotalBytes += n
}

// failed 记 n 个文件没传成。
func (a *transferAccount) failed(n int) { a.stat.FailFiles += n }

// failedRest 到点收场：没传成的都算没传成，已经逐文件记过的也在其中，因此是「照着总数算」
// 而不是往上再加——加会把那一个重复计两遍。
func (a *transferAccount) failedRest() {
	a.stat.FailFiles = a.stat.TotalFiles - a.stat.SuccessFiles
}

// allFailed 早退：这次打算传的一个都没传成。
func (a *transferAccount) allFailed() { a.stat.FailFiles = a.stat.TotalFiles }

// line 记一条明细：「出过事的那几个」都走这一处。
func (a *transferAccount) line(s string) { a.lines = append(a.lines, s) }

// settle 唯一出口：四个数与明细一起写进结果。
func (a *transferAccount) settle(res *contract.NodeResult) {
	res.Transfer = a.stat
	res.Output = buildTransferOutput(a.stat.TotalFiles, a.stat.SuccessFiles, a.stat.FailFiles, a.lines)
}
