package diagnose

import (
	"fmt"
	"strings"
	"testing"
)

// 字节构成与行尾形态是读日志时的判据，期望值按已知字节手写，不用被测算法反算。
func TestStatsAndEndings(t *testing.T) {
	cases := []struct {
		name                 string
		in                   string
		cr, lf               int
		esc, nul             int
		otherCtrl            int
		crlf, onlyLF, onlyCR int
	}{
		{"纯 LF 单行", "System in health state.\n", 0, 1, 0, 0, 0, 0, 1, 0},
		{"CRLF 单行", "System in health state.\r\n", 1, 1, 0, 0, 0, 1, 0, 0},
		{"孤立 CR", "a\rb\r", 2, 0, 0, 0, 0, 0, 0, 2},
		{"ANSI 颜色", "\x1b[32mOK\x1b[0m\n", 0, 1, 2, 0, 0, 0, 1, 0},
		{"退格与 NUL", "ab\b\x00", 0, 0, 0, 1, 1, 0, 0, 0},
		{"无结尾换行", "abc", 0, 0, 0, 0, 0, 0, 0, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			st := Stats(c.in)
			if st.CR != c.cr || st.LF != c.lf || st.ESC != c.esc || st.NUL != c.nul || st.OtherCtrl != c.otherCtrl {
				t.Fatalf("Stats(%q) = %+v，期望 CR=%d LF=%d ESC=%d NUL=%d 其它=%d",
					c.in, st, c.cr, c.lf, c.esc, c.nul, c.otherCtrl)
			}
			e := Endings(c.in)
			if e.CRLF != c.crlf || e.LF != c.onlyLF || e.CR != c.onlyCR {
				t.Fatalf("Endings(%q) = %+v，期望 CRLF=%d 只LF=%d 只CR=%d",
					c.in, e, c.crlf, c.onlyLF, c.onlyCR)
			}
		})
	}
}

// 无结尾换行的末行要单独计数：远端最后一行不带换行时，这一行最容易在别处出问题。
func TestEndingsCountsTrailingLineWithoutNewline(t *testing.T) {
	if got := Endings("a\nb").None; got != 1 {
		t.Fatalf("末行无换行应计 1，实际 %d", got)
	}
}

// 转义形式要能把回车、换行、ESC 写成可读记号，普通字符原样保留。
func TestEscaped(t *testing.T) {
	got := Escaped("a\r\nb\x1b[0m", 100)
	want := `a\r\nb\x1b[0m`
	if got != want {
		t.Fatalf("Escaped = %q，期望 %q", got, want)
	}
}

// 十六进制要能真正还原出字节（读日志的人靠它复核），且超长要截断并注明剩余量。
func TestHexDumpShowsBytesAndTruncates(t *testing.T) {
	dump := HexDump("ab\r\n", 512)
	if !strings.Contains(dump, "61 62 0d 0a") {
		t.Fatalf("十六进制应含 61 62 0d 0a，实际：\n%s", dump)
	}
	long := strings.Repeat("x", 40)
	dump = HexDump(long, 16)
	if !strings.Contains(dump, "省略 24 字节") {
		t.Fatalf("超长应注明剩余字节数，实际：\n%s", dump)
	}
}

// 记录器把三种事实分段落进日志；远端带 CR 时汇总要点名节点。
func TestRecorderWritesSectionsAndSummary(t *testing.T) {
	var out strings.Builder
	rec := New(writerLogger{&out})

	rec.RemoteRaw(0, "10.0.0.1", "System in health state.\r\n")
	rec.ResultDetail(0, "10.0.0.1", "【10.0.0.1】 分类: 执行成功\nSystem in health state.\n======")
	rec.Summary()

	log := out.String()
	for _, want := range []string{
		"远端原始输出（整备之前）",
		"CR=1 LF=1",
		"CRLF=1",
		`"System in health state.\r\n"`,
		"最终交给终端的文本",
		"汇总",
		"远端原始输出带 CR 的节点 1 个",
		"【10.0.0.1】CR=1",
	} {
		if !strings.Contains(log, want) {
			t.Fatalf("日志缺少 %q：\n%s", want, log)
		}
	}
	// 最终文本已经归一，汇总里「仍带控制字符」应为 0——这两行并排就是判据。
	if !strings.Contains(log, "最终文本仍带控制字符的节点 0 个") {
		t.Fatalf("汇总里最终文本不该再有控制字符：\n%s", log)
	}
}

// 同一节点重复登记原始输出只记第一次：worker 重试不该在日志里产生重复段。
func TestRecorderWritesRawOncePerNode(t *testing.T) {
	var out strings.Builder
	rec := New(writerLogger{&out})
	rec.RemoteRaw(0, "10.0.0.1", "a\r")
	rec.RemoteRaw(0, "10.0.0.1", "a\r")
	if n := strings.Count(out.String(), "远端原始输出（整备之前）"); n != 1 {
		t.Fatalf("同一节点应只记一次，实际 %d 次", n)
	}
}

// writerLogger 把诊断日志写进缓冲（诊断器只用到 Info）。
type writerLogger struct{ b *strings.Builder }

func (w writerLogger) Info(args ...any) {
	for _, a := range args {
		w.b.WriteString(fmt.Sprint(a))
	}
	w.b.WriteString("\n")
}
