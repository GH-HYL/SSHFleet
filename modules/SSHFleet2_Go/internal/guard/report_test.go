package guard

import (
	"strings"
	"testing"
)

// 报错四样缺一不可：类名、位置、哪里绕开了、该走哪一处；开头还要说清这是什么红。
func TestReportKeepsFourParts(t *testing.T) {
	v := Violation{Class: ClassExit, File: "internal/a/b.go", Line: 3, What: "调了 os.Exit", Why: "退出只许塔尖", Fix: "交给塔尖"}
	text := Report([]Violation{v})
	for _, want := range []string{FenceNote, ClassExit, "internal/a/b.go:3", "调了 os.Exit", "退出只许塔尖", "交给塔尖"} {
		if !strings.Contains(text, want) {
			t.Errorf("报错里少了 %q：\n%s", want, text)
		}
	}
	if Report(nil) != "" {
		t.Error("零违规应当返回空串")
	}
}
