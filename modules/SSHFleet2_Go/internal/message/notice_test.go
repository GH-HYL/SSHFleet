package message

// 解析站路上那几句提示的逐字判据（这里只有上传那一条；其余在 `notice.go`）。
//
// 期望串照老版真跑：日志里那一行见 `build/建站05_18_2026-11-03_1445/out/03b-upload-clean/old.history/SSHFleetExec.log`，
// 交互路径「开始执行」之后那一行见 `out/21-upload-yes/old.screen`。

import (
	"strings"
	"testing"
)

// TestUploadSkippedText 名单位置、条数与措辞：一句提示，进日志。
func TestUploadSkippedText(t *testing.T) {
	got := UploadSkippedText([]string{"link.txt"})
	want := "[提示] 上传源中有 1 个软链接/快捷方式被过滤（不上传）：link.txt"
	if got != want {
		t.Fatalf("那句话说错\n got=%q\nwant=%q", got, want)
	}
}

// TestUploadSkippedTextTruncates 名单超过五个就截断：清单可能几百个，一句提示刷满屏没有意义。
func TestUploadSkippedTextTruncates(t *testing.T) {
	got := UploadSkippedText([]string{"a", "b", "c", "d", "e", "f", "g"})
	if !strings.Contains(got, "有 7 个") {
		t.Fatalf("条数该报总数：%q", got)
	}
	if !strings.HasSuffix(got, "：a、b、c、d、e 等 7 个") {
		t.Fatalf("名单该只列五个再写「等 N 个」：%q", got)
	}
}

// TestUploadSkippedLine 屏幕那一行：与日志同一句话，末尾带换行。
func TestUploadSkippedLine(t *testing.T) {
	got := UploadSkippedLine([]string{"link.txt"}).Text()
	want := "[提示] 上传源中有 1 个软链接/快捷方式被过滤（不上传）：link.txt\n"
	if got != want {
		t.Fatalf("屏幕上那一行说错\n got=%q\nwant=%q", got, want)
	}
}
