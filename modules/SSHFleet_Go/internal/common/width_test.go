package common

import (
	"strings"
	"testing"
)

func TestDisplayWidth(t *testing.T) {
	cases := []struct {
		in   string
		want int
	}{
		{"", 0},
		{"abc", 3},
		{"中文", 4},
		{"中a", 3},
		{"（全角括号）", 12},
		{"═", 1},      // 制表符系（EastAsianAmbiguous）按 1 列，与旧版 display_width 一致
		{"⚠️", 2},     // 基础码位 1 + 变体选择符 1 —— 恰好等于终端按 emoji 渲染的 2 列
		{"🚫", 2},      // EastAsianWide
		{"版本: v1", 8}, // 中(2)本(2):(1)空格(1)v(1)1(1)
	}
	for _, c := range cases {
		if got := DisplayWidth(c.in); got != c.want {
			t.Fatalf("DisplayWidth(%q) 应为 %d，实际 %d", c.in, c.want, got)
		}
	}
}

func TestTrimToWidth(t *testing.T) {
	if got := TrimToWidth("abcdef", 3); got != "abc" {
		t.Fatalf("应为 abc，实际 %q", got)
	}
	if got := TrimToWidth("中文中文", 5); got != "中文" { // 5 列放不下第三个汉字
		t.Fatalf("应为「中文」，实际 %q", got)
	}
	if got := TrimToWidth("abc", 10); got != "abc" {
		t.Fatalf("够宽应原样返回，实际 %q", got)
	}
}

func TestWrapByWidth(t *testing.T) {
	t.Run("够宽不折行", func(t *testing.T) {
		got := WrapByWidth("短文本", 40)
		if len(got) != 1 || got[0] != "短文本" {
			t.Fatalf("应为单行原文，实际 %v", got)
		}
	})

	t.Run("width<=0 不折行", func(t *testing.T) {
		long := strings.Repeat("很长的一段说明", 8)
		got := WrapByWidth(long, 0)
		if len(got) != 1 || got[0] != long {
			t.Fatalf("width<=0 应原样单行返回")
		}
	})

	t.Run("每行不超宽且内容不丢", func(t *testing.T) {
		text := "转换凭据文件（后面跟目标文件路径）：自动识别明文/base64/加密格式并按配置等级转换，支持升降级；加密/解密需已配置主密钥"
		for _, width := range []int{8, 16, 24, 30, 40, 61} {
			lines := WrapByWidth(text, width)
			var got strings.Builder
			for _, ln := range lines {
				if w := DisplayWidth(ln); w > width {
					t.Fatalf("width=%d 时某行 %d 列超宽：%q", width, w, ln)
				}
				got.WriteString(strings.ReplaceAll(ln, " ", ""))
			}
			want := strings.ReplaceAll(text, " ", "")
			if got.String() != want {
				t.Fatalf("width=%d 折行后内容丢失\n得到：%q\n期望：%q", width, got.String(), want)
			}
		}
	})

	t.Run("优先在空格处断", func(t *testing.T) {
		got := WrapByWidth("aaa bbb ccc", 7)
		if len(got) != 2 || got[0] != "aaa bbb" || got[1] != "ccc" {
			t.Fatalf("应在空格处断行，实际 %v", got)
		}
		if strings.HasPrefix(got[1], " ") {
			t.Fatalf("续行不应以空格开头：%q", got[1])
		}
	})

	t.Run("单字符超宽不死循环", func(t *testing.T) {
		got := WrapByWidth("中文abc", 1)
		if len(got) != 5 { // 中(2列) 文(2列) 各占一行，a b c 各一行
			t.Fatalf("width=1 时应每字符一行，实际 %v", got)
		}
		for _, ln := range got {
			if ln == "" {
				t.Fatalf("不应出现空行：%v", got)
			}
		}
	})
}
