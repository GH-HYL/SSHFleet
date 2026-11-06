package parse

import "testing"

// _test.go 不在类七的射程：测试里造一句带星号的样本不算越界。

func TestScrubSample(t *testing.T) {
	if got := Scrub("pw=secret", "secret"); got != "pw=****" {
		t.Fatalf("样本自检失败：%q", got)
	}
}
