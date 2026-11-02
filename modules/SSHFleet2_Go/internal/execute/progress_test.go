package execute

// 进度面板与屏幕那一路：只判形状与数字，不判颜色（颜色归终端与降色，测试里没有终端）。

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

// TestProgressViewShowsCounts 面板正文里该有的数与字：总进度、已完成、耗时、成功失败。
func TestProgressViewShowsCounts(t *testing.T) {
	m := newProgressModel(3, time.Now().Add(-65*time.Second))
	m.completed, m.succeeded, m.failed = 2, 1, 1

	got := m.render()
	for _, want := range []string{"总进度", "已完成 2/3", "0:01:05", "成功 1", "失败 1", "━"} {
		if !strings.Contains(got, want) {
			t.Fatalf("面板里少了 %q：\n%s", want, got)
		}
	}
}

// TestProgressPercent 进度按台数算，封顶 1，没开工是 0（不许先冲满再回落）。
func TestProgressPercent(t *testing.T) {
	m := newProgressModel(4, time.Now())
	if got := m.percent(); got != 0 {
		t.Fatalf("没开工时进度该是 0，实际 %v", got)
	}
	m.completed = 2
	if got := m.percent(); got != 0.5 {
		t.Fatalf("跑完两台该是一半，实际 %v", got)
	}
	m.completed = 9
	if got := m.percent(); got != 1 {
		t.Fatalf("超出总台数时封顶 1，实际 %v", got)
	}
}

// TestElapsedText 耗时写成 `0:01:05`（老版同一格式）。
func TestElapsedText(t *testing.T) {
	cases := []struct {
		d    time.Duration
		want string
	}{
		{0, "0:00:00"},
		{65 * time.Second, "0:01:05"},
		{3725 * time.Second, "1:02:05"},
	}
	for _, c := range cases {
		if got := elapsedText(c.d); got != c.want {
			t.Fatalf("%v 该写成 %q，实际 %q", c.d, c.want, got)
		}
	}
}

// TestScreenPlainWritesStraight 不是终端时不画界面：结果行直接落流，一个字都不多。
func TestScreenPlainWritesStraight(t *testing.T) {
	var buf bytes.Buffer
	scr := newScreen(&buf)
	if !scr.plain {
		t.Skip("这会儿标准输出连着终端，直通那一路测不了")
	}

	scr.above("【10.0.0.1】 连接: 成功 - 0.123s\n")
	scr.progress(1, 1, 0, 1, time.Now()) // 直通：不画界面
	scr.stop()

	if got, want := buf.String(), "【10.0.0.1】 连接: 成功 - 0.123s\n"; got != want {
		t.Fatalf("直通那一路应原样落流。\n期望：%q\n实际：%q", want, got)
	}
}
