package execute

// 空间预检：只有目标目录这一侧，加上两种降级（ADR-0026 的暂存侧已随提权撤掉）。
//
// 探不到数不该挡住上传——这是 ADR-0026 的取向：预检是帮忙，不是门禁。这里用假探测
// 把那几种组合钉住，真机那条（小卷装不下）在 `run_upload_space_integration_test.go`。

import (
	"strings"
	"testing"
)

// probeOf 造一个按路径回答的探测器：路径在 map 里就当探得到，不在就探不到；
// 第二个参数是「按 root 视角问」，这里只记下来，不影响回答。
func probeOf(avail map[string]int64) func(string, bool) (int64, bool) {
	return func(p string, _ bool) (int64, bool) {
		v, ok := avail[p]
		return v, ok
	}
}

// 够就一个字都不说。
func TestSpaceShortSilentWhenEnough(t *testing.T) {
	probe := probeOf(map[string]int64{"/target": 1000})
	if m := spaceShort(probe, "/target", false, 500, true); m != "" {
		t.Fatalf("装得下却说话了：%q", m)
	}
}

// 不够就报目标分区，报文里带「没写文件」那句话；提权与非提权同一句话。
func TestSpaceShortTargetShort(t *testing.T) {
	for _, asRoot := range []bool{false, true} {
		probe := probeOf(map[string]int64{"/target": 100})
		m := spaceShort(probe, "/target", asRoot, 500, true)
		if !strings.HasPrefix(m, "sshfleet.transfer.target_space_short: ") {
			t.Fatalf("该报目标分区装不下（asRoot=%v），实际 %q", asRoot, m)
		}
		if !strings.Contains(m, "no file written") {
			t.Fatalf("预检那一刻还没写文件，报文里要说清，实际 %q", m)
		}
		if strings.Contains(m, "left to send") {
			t.Fatalf("预检不该用「还差多少」那种说法：%q", m)
		}
	}
}

// 探不到数：等于没有预检，绝不因此挡住上传。
func TestSpaceShortDegradesWhenProbeFails(t *testing.T) {
	probe := probeOf(map[string]int64{})
	if m := spaceShort(probe, "/target", true, 1<<40, true); m != "" {
		t.Fatalf("探不到时该当作不判，却报了 %q", m)
	}
}

// 失败之后那一问：同一处不够，措辞换成「还差多少」，不说「没写文件」。
func TestSpaceShortAfterFailureWording(t *testing.T) {
	probe := probeOf(map[string]int64{"/target": 100})
	m := spaceShort(probe, "/target", false, 500, false)
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
	m := spaceShort(probe, "/target/", false, 500, true)
	if !strings.Contains(m, "/target: ") {
		t.Fatalf("尾斜杠没去掉，实际 %q", m)
	}
}
