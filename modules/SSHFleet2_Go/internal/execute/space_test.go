package execute

// 空间预检的两侧与两种降级（13 号单）。
//
// 探不到数不该挡住上传——这是 ADR-0026 的取向：预检是帮忙，不是门禁。这里用假探测
// 把那几种组合钉住，真机那条（小卷装不下）在 `run_upload_space_integration_test.go`。

import (
	"strings"
	"testing"
)

// probeOf 造一个按路径回答的探测器：路径在 map 里就当探得到，不在就探不到。
func probeOf(avail map[string]int64) func(string, bool) (int64, bool) {
	return func(p string, _ bool) (int64, bool) {
		v, ok := avail[p]
		return v, ok
	}
}

// 非 sudo：只看目标侧，够就一个字都不说。
func TestSpaceShortSilentWhenEnough(t *testing.T) {
	probe := probeOf(map[string]int64{"/target": 1000})
	if m := spaceShort(probe, "/target", false, 500, 0, true); m != "" {
		t.Fatalf("装得下却说话了：%q", m)
	}
}

// 非 sudo：目标侧不够就报目标侧，报文里带「没写文件」那句话。
func TestSpaceShortTargetShortNonSudo(t *testing.T) {
	probe := probeOf(map[string]int64{"/target": 100})
	m := spaceShort(probe, "/target", false, 500, 0, true)
	if !strings.HasPrefix(m, "sshfleet.transfer.target_space_short: ") {
		t.Fatalf("该报目标分区装不下，实际 %q", m)
	}
	if !strings.Contains(m, "no file written") {
		t.Fatalf("预检那一刻还没写文件，报文里要说清，实际 %q", m)
	}
	if strings.Contains(m, "left to send") {
		t.Fatalf("预检不该用「还差多少」那种说法：%q", m)
	}
}

// 探不到数：少判一侧，绝不因此挡住上传。
func TestSpaceShortDegradesWhenProbeFails(t *testing.T) {
	probe := probeOf(map[string]int64{})
	if m := spaceShort(probe, "/target", true, 1<<40, 1<<40, true); m != "" {
		t.Fatalf("两侧都探不到时等于没有预检，却报了 %q", m)
	}
}

// sudo：目标侧够、暂存侧不够 ⇒ 报的是暂存那一侧，不是目标侧。
//
// 两侧各按自己的峰值判：目标侧按本次总量、暂存侧按最大单文件，所以可以有「总量装得下、
// 单个文件在 /tmp 放不下」这种组合。
func TestSpaceShortTmpShortOnlyOnSudo(t *testing.T) {
	tmpPath := tmpProbePath()
	probe := probeOf(map[string]int64{"/target": 1 << 30, tmpPath: 10})

	m := spaceShort(probe, "/target", true, 1000, 5000, true)
	if !strings.HasPrefix(m, "sshfleet.transfer.tmp_space_short: ") {
		t.Fatalf("该报暂存分区装不下，实际 %q", m)
	}
	// 非 sudo 没有暂存这一步：同一组数只该看目标侧，一个字都不说。
	if m := spaceShort(probe, "/target", false, 1000, 5000, true); m != "" {
		t.Fatalf("非 sudo 不该判暂存侧，实际 %q", m)
	}
}

// 失败之后那一问：同一处不够，措辞换成「还差多少」，不说「没写文件」。
func TestSpaceShortAfterFailureWording(t *testing.T) {
	probe := probeOf(map[string]int64{"/target": 100})
	m := spaceShort(probe, "/target", false, 500, 0, false)
	if strings.Contains(m, "no file written") {
		t.Fatalf("已经写过了，不该再说「没写文件」：%q", m)
	}
	if !strings.Contains(m, "left to send") {
		t.Fatalf("该说还差多少，实际 %q", m)
	}
}

// 目标路径带尾斜杠时，报文里不出现「/target/:」这种写法。
func TestSpaceShortDetailTrimsTrailingSlash(t *testing.T) {
	probe := probeOf(map[string]int64{"/target/": 100})
	m := spaceShort(probe, "/target/", false, 500, 0, true)
	if !strings.Contains(m, "/target: ") {
		t.Fatalf("尾斜杠没去掉，实际 %q", m)
	}
}
