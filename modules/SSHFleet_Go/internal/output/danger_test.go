package output

import (
	"regexp"
	"strings"
	"testing"

	"sshfleet/internal/dangercheck"
)

// 危险命令警告框的对齐回归：框内必须逐行等宽，否则右边框呈锯齿
// （用户 2026-09-15 反馈「符号没有对齐」）。

var ansiRe = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func stripANSI(s string) string { return ansiRe.ReplaceAllString(s, "") }

func dangerReport(content, rule string, level string) *dangercheck.Report {
	return &dangercheck.Report{Matches: []dangercheck.Match{
		{Line: 1, Content: content, RuleName: rule, RiskLevel: level},
	}}
}

func assertEqualLineWidth(t *testing.T, box string, forbidden bool) {
	t.Helper()
	want := dangerInnerWidth + 2
	for i, ln := range strings.Split(box, "\n") {
		plain := stripANSI(ln)
		if got := DisplayWidth(plain); got != want {
			t.Fatalf("forbidden=%v 第 %d 行显示宽度 %d，应为 %d：%q", forbidden, i+1, got, want, plain)
		}
	}
}

func TestDangerBoxLinesAreEqualWidth(t *testing.T) {
	report := dangerReport("rm -rf CHANGELOG.md multipath.conf README.md SSHFleet_windows.zip", "递归强制删除", "high")
	assertEqualLineWidth(t, dangerBox(report, false), false)
	assertEqualLineWidth(t, dangerBox(report, true), true)
}

// 中文内容按显示宽度截断：按字符数截断会让 46 个汉字撑到 92 列，框线必歪。
func TestDangerBoxTruncatesLongCJKByDisplayWidth(t *testing.T) {
	report := dangerReport(strings.Repeat("中", 80), strings.Repeat("删", 80), "high")
	assertEqualLineWidth(t, dangerBox(report, false), false)

	// 截断到 46 列 → 23 个汉字
	plain := stripANSI(dangerBox(report, false))
	for _, ln := range strings.Split(plain, "\n") {
		if strings.Contains(ln, "内容:") {
			body := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(ln), "║"))
			body = strings.TrimPrefix(strings.TrimSpace(body), "内容:")
			body = strings.TrimSuffix(strings.TrimSpace(body), "║")
			if got := DisplayWidth(strings.TrimSpace(body)); got != dangerFieldLimit {
				t.Fatalf("中文内容应截断到 %d 列，实际 %d 列", dangerFieldLimit, got)
			}
		}
	}
}

// 标题两侧留白对称（旧版右侧少一个空格，看着像「前后少个空格」）。
func TestDangerBoxTitlePaddingIsSymmetric(t *testing.T) {
	for _, forbidden := range []bool{false, true} {
		plain := stripANSI(dangerBox(dangerReport("rm -rf /", "递归强制删除", "high"), forbidden))
		var titleLine string
		for _, ln := range strings.Split(plain, "\n") {
			if strings.Contains(ln, "发现") {
				titleLine = ln
				break
			}
		}
		if titleLine == "" {
			t.Fatalf("forbidden=%v 未找到标题行", forbidden)
		}
		inner := []rune(titleLine)[1 : len([]rune(titleLine))-1]
		left, right := 0, 0
		for left < len(inner) && inner[left] == ' ' {
			left++
		}
		for right < len(inner) && inner[len(inner)-1-right] == ' ' {
			right++
		}
		if left != right {
			t.Fatalf("forbidden=%v 标题左右留白不对称：左 %d 右 %d", forbidden, left, right)
		}
		// 标题文本内部两侧的空格数也应一致
		text := string(inner[left : len(inner)-right])
		if m := regexp.MustCompile(`^(.*?)( +)(.*?)( +)(\S.*\S|\S)$`).FindStringSubmatch(text); m != nil {
			if len(m[2]) != len(m[4]) {
				t.Fatalf("forbidden=%v 标题文本两侧空格不一致：左 %d 右 %d（%q）", forbidden, len(m[2]), len(m[4]), text)
			}
		} else {
			t.Fatalf("forbidden=%v 标题未解析出两侧空格：%q", forbidden, text)
		}
	}
}

// 空报告不产生任何输出。
func TestDangerBoxEmptyReport(t *testing.T) {
	if got := dangerBox(&dangercheck.Report{}, false); got != "" {
		t.Fatalf("无命中应返回空串，实际：%q", got)
	}
	if got := dangerBox(nil, false); got != "" {
		t.Fatalf("nil 报告应返回空串，实际：%q", got)
	}
}

// 变体选择符（U+FE0F）是零宽的：「⚠️」（U+26A0 + U+FE0F）与「⚠」在终端里画出来
// 一样宽，宽度函数也必须给一样的值。此前前者被算成 2 列、后者 1 列，标题因此
// 多算 2 列、居中偏移、右边框歪掉（用户 2026-09-15 反馈「两个感叹号没对齐」）。
// 上面那条「逐行等宽」断言查不出这个——它用的是同一个宽度函数，自己证自己。
func TestVariantSelectorCountsAsZeroWidth(t *testing.T) {
	if got := DisplayWidth("⚠"); got != 1 {
		t.Fatalf("「⚠」应为 1 列，实际 %d", got)
	}
	if got := DisplayWidth("⚠️"); got != 1 {
		t.Fatalf("「⚠️」应也为 1 列（U+FE0F 零宽），实际 %d", got)
	}
	// 标题里不再出现带变体选择符的写法，避免终端按 emoji 呈现时宽度不可预期
	for _, forbidden := range []bool{false, true} {
		if strings.Contains(stripANSI(dangerBox(dangerReport("rm -rf /", "递归强制删除", "high"), forbidden)), "\ufe0f") {
			t.Fatalf("forbidden=%v 标题不应使用变体选择符", forbidden)
		}
	}
}
