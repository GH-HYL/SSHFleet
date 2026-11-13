package parse

import (
	"strings"
	"testing"
)

// 二进制判定：只看有没有 NUL，且看全文——老版的前 1KB 是性能取舍，不是判据。

func TestIsBinaryContent(t *testing.T) {
	for _, bin := range [][]byte{
		{0},
		[]byte("abc\x00def"),
		append([]byte(strings.Repeat("a", 1500)), 0), // 前 1KB 干净，NUL 在后面
	} {
		if !isBinaryContent(bin) {
			t.Errorf("该判成二进制：%q…", bin[:min(len(bin), 16)])
		}
	}
	for _, txt := range [][]byte{
		nil,
		{},
		[]byte("10.0.0.1,22,root,pw\n"),
		[]byte("\n\r\t \u00e9\u4e2d"),
	} {
		if isBinaryContent(txt) {
			t.Errorf("不该判成二进制：%q", txt)
		}
	}
}
