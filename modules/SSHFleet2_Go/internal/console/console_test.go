package console

import (
	"io"
	"os"
	"testing"

	"sshfleet2/internal/message"
)

// 把标准输出接到管道再问：管道不是终端，宽度一律 0（「不折行」）。
// 真终端下宽度大于 0 那一条要在真机手工跑一次，结论写进工单 Comments——跳过不算绿。
func TestWidthOnPipeIsZero(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("起管道失败：%v", err)
	}
	old := os.Stdout
	os.Stdout = w
	got := Width()
	os.Stdout = old
	_ = w.Close()
	_, _ = io.Copy(io.Discard, r)
	_ = r.Close()

	if got != 0 {
		t.Errorf("管道下宽度应为 0，got %d", got)
	}
}

// Print 原样把话送到标准输出，一个字不改。
func TestPrintWritesVerbatim(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("起管道失败：%v", err)
	}
	old := os.Stdout
	os.Stdout = w
	// 话造不出来，只能向 message 要一句——这正是这次要的那道闸：console 的测试
	// 也得走 message，绕不过去。
	say := message.NotWiredLine()
	Print(say)
	_ = w.Close()
	os.Stdout = old
	data, rerr := io.ReadAll(r)
	_ = r.Close()
	if rerr != nil {
		t.Fatalf("读回失败：%v", rerr)
	}
	if string(data) != say.Text() {
		t.Errorf("Print 应原样写出：want %q, got %q", say.Text(), string(data))
	}
}
