package execute

// 哪些失败值得再来一次（ADR-0063 决定 10）。

import (
	"errors"
	"testing"

	"github.com/pkg/sftp"
)

func TestRetryableTransferError(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"读写中断：重来有意义", errors.New("connection reset by peer"), true},
		{"大小不符：重来有意义", errors.New(sizeMismatchMessage(2048, 1024)), true},
		{"没有权限：重来还是同一个结果", &sftp.StatusError{Code: uint32(sftp.ErrSSHFxPermissionDenied)}, false},
		{"文件不存在：同上", &sftp.StatusError{Code: uint32(sftp.ErrSSHFxNoSuchFile)}, false},
		{"操作不支持：同上", &sftp.StatusError{Code: uint32(sftp.ErrSSHFxOpUnsupported)}, false},
		{"笼统的 failure：认不出来，按可重来算", &sftp.StatusError{Code: uint32(sftp.ErrSSHFxFailure)}, true},
		{"本机读不了：重来还是读不了", noRetry(errors.New("open /x: permission denied")), false},
		{"包了一层也认得出原始状态", transferFail(transferRemoteFileFailed, &sftp.StatusError{Code: uint32(sftp.ErrSSHFxPermissionDenied)}), false},
	}
	for _, c := range cases {
		if got := retryableTransferError(c.err); got != c.want {
			t.Errorf("%s：want %v，got %v（%v）", c.name, c.want, got, c.err)
		}
	}
}

// 报文只说两个数，不带结论。
func TestSizeMismatchMessage(t *testing.T) {
	got := sizeMismatchMessage(2048, 1024)
	if want := "sshfleet.transfer.size_mismatch: local 2048 bytes, remote 1024 bytes"; got != want {
		t.Fatalf("want %q，got %q", want, got)
	}
}
