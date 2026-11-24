package execute

// 传输没跑成时的记账口径（2026-11-17 架构复查 03 号单）。
//
// 上传与下载的早退路合起来十来条，以前各写一遍：新增一条路径忘写一处，报告里的文件数就印错；
// 下载的到点收场更会把已经传成的成功数与字节当成 0 交出去。这几条钉住那本账唯一出口的口径。

import (
	"testing"

	"sshfleet2/internal/contract"
)

// 一个都没传成：打算传几个，就记几个失败。
func TestTransferAccountMarksEveryPlannedFile(t *testing.T) {
	res := &contract.NodeResult{}
	acct := newTransferAccount(3, nil)

	acct.allFailed()
	acct.settle(res)

	if res.Transfer.TotalFiles != 3 || res.Transfer.FailFiles != 3 {
		t.Fatalf("该记 3 个打算传、3 个失败，实际 total=%d fail=%d",
			res.Transfer.TotalFiles, res.Transfer.FailFiles)
	}
	if res.Transfer.SuccessFiles != 0 {
		t.Fatalf("早退时一个都没传成，成功数该是 0，实际 %d", res.Transfer.SuccessFiles)
	}
}

// 枚举之前就失败（连不上 sftp、远路径不在）：没有文件可数，记 0 而不是编一个数。
func TestTransferAccountWithNothingEnumerated(t *testing.T) {
	res := &contract.NodeResult{}
	acct := newTransferAccount(0, nil)

	acct.allFailed()
	acct.settle(res)

	if res.Transfer.TotalFiles != 0 || res.Transfer.FailFiles != 0 {
		t.Fatalf("没枚举到文件时该记 0/0，实际 total=%d fail=%d",
			res.Transfer.TotalFiles, res.Transfer.FailFiles)
	}
}
