package common

import (
	"strings"
	"testing"
)

// 三行齐：两处标签都在，顺序是问题 → 原因 → 提示。
func TestErrUserBuildsThreeLines(t *testing.T) {
	err := ErrUser("值读不到：/x", "no such file or directory", "换一个路径")
	want := "值读不到：/x\n原因：no such file or directory\n提示：换一个路径"
	if got := err.Error(); got != want {
		t.Fatalf("文案应为 %q，实际：%q", want, got)
	}
}

// 只有原因：没有提示行。
func TestErrUserReasonHasNoHintLine(t *testing.T) {
	err := ErrUserReason("读取配置失败：/x", "permission denied")
	want := "读取配置失败：/x\n原因：permission denied"
	if got := err.Error(); got != want {
		t.Fatalf("文案应为 %q，实际：%q", want, got)
	}
	if strings.Contains(err.Error(), "\n提示：") {
		t.Fatalf("不该有提示行：%q", err.Error())
	}
}

// 只有提示：没有原因行。
func TestErrUserShortHasNoReasonLine(t *testing.T) {
	err := ErrUserShort("-p 指的路径不是目录：/x", "指向一个目录")
	want := "-p 指的路径不是目录：/x\n提示：指向一个目录"
	if got := err.Error(); got != want {
		t.Fatalf("文案应为 %q，实际：%q", want, got)
	}
	if strings.Contains(err.Error(), "\n原因：") {
		t.Fatalf("不该有原因行：%q", err.Error())
	}
}

// 内部错误的第三行恒定，不由调用点给。
func TestErrInternalHintIsFixed(t *testing.T) {
	err := ErrInternal("上传清单缺失", "主干未在上传模式下采集清单")
	wantHint := "\n提示：这是内部错误，请把本次命令反馈给作者"
	if !strings.HasSuffix(err.Error(), wantHint) {
		t.Fatalf("末行应为 %q，实际：%q", wantHint, err.Error())
	}
}

// 构造点不改内容：首尾空白、结尾换行都原样留着。
func TestErrTextKeepsContentAsIs(t *testing.T) {
	err := ErrUser(" 前有空格", " 后也有空格", "尾随换行\n")
	want := " 前有空格\n原因： 后也有空格\n提示：尾随换行\n"
	if got := err.Error(); got != want {
		t.Fatalf("文案应为 %q，实际：%q", want, got)
	}
}
