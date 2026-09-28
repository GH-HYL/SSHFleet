package ssh

import (
	"strings"
	"testing"
)

// 匹配器是纯函数化的一层（给定字节流 + 触发词表 → 命中序列），这一组测试覆盖
// 它必须成立的几条：游标只在命中时前进、并行取最先出现者、不按行匹配、
// 大小写与正则开关、已送出的条目不重复命中。

// 触发词被读边界切成两半时，游标不动，后半截到齐后自然命中。
func TestMatcherHitAcrossChunks(t *testing.T) {
	m := newCursorMatcher([][]string{{"请选择架构"}}, MatchOptions{})

	if hits := m.feed("请选择"); len(hits) != 0 {
		t.Fatalf("半个提示不该命中，实际：%v", hits)
	}
	hits := m.feed("架构 [1] ")
	if len(hits) != 1 || hits[0].rule != 0 {
		t.Fatalf("后半截到齐后应命中第 1 条，实际：%v", hits)
	}
	if len(m.pending()) != 0 {
		t.Fatalf("命中后不该仍有未送出条目：%v", m.pending())
	}
}

// 一次推来的块里连着两条提示：命中第一条后游标推进到触发词末尾，第二条接着命中。
func TestMatcherTwoHitsInOneChunk(t *testing.T) {
	m := newCursorMatcher([][]string{{"架构"}, {"包格式"}}, MatchOptions{})

	hits := m.feed("请选择架构 [1] 然后选择包格式 [deb]")
	if len(hits) != 2 {
		t.Fatalf("同一块里的两条提示都应命中，实际 %d 条：%v", len(hits), hits)
	}
	if hits[0].rule != 0 || hits[1].rule != 1 {
		t.Fatalf("命中顺序应为 0 → 1，实际：%v", hits)
	}
}

// 多条代填并行参与匹配，取输出里最先出现的那条，与命令行给出的顺序无关。
func TestMatcherTakesEarliestNotFirstDefined(t *testing.T) {
	m := newCursorMatcher([][]string{{"架构"}, {"包格式"}}, MatchOptions{})

	hits := m.feed("包格式？")
	if len(hits) != 1 || hits[0].rule != 1 {
		t.Fatalf("应先命中位置最靠前的第 2 条，实际：%v", hits)
	}
}

// 提示常常不带换行（read -p 的提示就没有），匹配不能按行切分。
func TestMatcherMatchesWithoutNewline(t *testing.T) {
	m := newCursorMatcher([][]string{{"PROMPT_ARCH"}}, MatchOptions{})

	hits := m.feed("PROMPT_ARCH: ")
	if len(hits) != 1 {
		t.Fatalf("无换行的提示也应命中，实际：%v", hits)
	}
}

// 已命中的条目不再参与匹配——一趟运行里一条代填只送一次。
func TestMatcherDoesNotRepeatHit(t *testing.T) {
	m := newCursorMatcher([][]string{{"架构"}}, MatchOptions{})

	if hits := m.feed("架构"); len(hits) != 1 {
		t.Fatalf("首次应命中，实际：%v", hits)
	}
	if hits := m.feed("再问一次架构"); len(hits) != 0 {
		t.Fatalf("已送出的条目不该再命中，实际：%v", hits)
	}
}

// 大小写：默认不区分（与判据文件现行口径一致），开关打开后区分。
func TestMatcherCaseSensitive(t *testing.T) {
	insensitive := newCursorMatcher([][]string{{"Please choose"}}, MatchOptions{})
	if hits := insensitive.feed("PLEASE CHOOSE: "); len(hits) != 1 {
		t.Fatalf("默认应不区分大小写，实际：%v", hits)
	}

	sensitive := newCursorMatcher([][]string{{"Please choose"}}, MatchOptions{CaseSensitive: true})
	if hits := sensitive.feed("PLEASE CHOOSE: "); len(hits) != 0 {
		t.Fatalf("区分大小写时不该命中，实际：%v", hits)
	}
	if hits := sensitive.feed("Please choose: "); len(hits) != 1 {
		t.Fatalf("原文一致时应命中，实际：%v", hits)
	}
}

// 正则开关：开启后关键词按正则解释，且同样默认不区分大小写。
func TestMatcherRegexMode(t *testing.T) {
	m := newCursorMatcher([][]string{{`choose .*arch`}}, MatchOptions{Regex: true})

	hits := m.feed("CHOOSE THE ARCH (x86_64/arm64): ")
	if len(hits) != 1 {
		t.Fatalf("正则模式应命中，实际：%v", hits)
	}
	if hits[0].keyword != `choose .*arch` {
		t.Fatalf("命中的关键词应原样报出，实际：%q", hits[0].keyword)
	}
}

// 零宽命中（如 a*）不推进游标，必须被丢掉——否则原地死循环。
func TestMatcherRejectsZeroWidthRegex(t *testing.T) {
	m := newCursorMatcher([][]string{{`x*`}}, MatchOptions{Regex: true})

	if hits := m.feed("abc"); len(hits) != 0 {
		t.Fatalf("零宽命中应被丢弃，实际：%v", hits)
	}
}

// pending 报未命中的序号（1 起），供收尾文案用。
func TestMatcherPendingIndexes(t *testing.T) {
	m := newCursorMatcher([][]string{{"架构"}, {"包格式"}, {"服务端口"}}, MatchOptions{})

	m.feed("请选择包格式")
	got := m.pending()
	want := []int{1, 3}
	if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("未命中序号应为 %v，实际 %v", want, got)
	}
}

// 命中行取整行原文，去掉行尾 \r——中止词的 error 文案要用它。
func TestMatcherHitLine(t *testing.T) {
	m := newCursorMatcher([][]string{{"Your password has expired"}}, MatchOptions{})

	hits := m.feed("WARNING: Your password has expired.\r\nPassword change required\r\n")
	if len(hits) != 1 {
		t.Fatalf("应命中，实际：%v", hits)
	}
	if hits[0].line != "WARNING: Your password has expired." {
		t.Fatalf("命中行应取整行原文，实际：%q", hits[0].line)
	}
}

// 非法正则在本层是程序内不变式（参数合规阶段已拦），ValidateKeywords 负责给出人话报错。
func TestValidateKeywords(t *testing.T) {
	if err := ValidateKeywords([]string{"架构"}, MatchOptions{}); err != nil {
		t.Fatalf("子串模式下不该校验正则：%v", err)
	}
	if err := ValidateKeywords([]string{`arch.*`}, MatchOptions{Regex: true}); err != nil {
		t.Fatalf("合法正则不该报错：%v", err)
	}
	err := ValidateKeywords([]string{`arch[`}, MatchOptions{Regex: true})
	if err == nil {
		t.Fatal("非法正则应报错")
	}
	if !strings.Contains(err.Error(), "arch[") {
		t.Fatalf("报错应带上触发词原文：%v", err)
	}
}
