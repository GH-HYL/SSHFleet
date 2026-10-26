package ssh

import (
	"testing"

	"github.com/pkg/sftp"
)

// 上传前判满的判据与报文（工单 01）。
//
// 这三条都是纯函数，期望值一律写成手算的字面量——不拿被测代码再算一遍，
// 否则按构造必然通过，判满判反了也发现不了。

func TestJudgeSpace(t *testing.T) {
	cases := []struct {
		name        string
		side        spaceSide
		path        string
		avail       int64
		need        int64
		beforeWrite bool
		wantShort   bool
		wantMsg     string
	}{
		{
			name: "目标侧：可用刚好等于需要——不算满（硬比，无余量）",
			side: sideTarget, path: "/data/app",
			avail: 2 << 30, need: 2 << 30, beforeWrite: true,
			wantShort: false, wantMsg: "",
		},
		{
			name: "目标侧：少一个字节就判满",
			side: sideTarget, path: "/data/app",
			avail: 2<<30 - 1, need: 2 << 30, beforeWrite: true,
			wantShort: true,
			wantMsg:   "sshfleet.transfer.target_space_short: /data/app: 2 GB available, 2 GB needed; no file written",
		},
		{
			name: "目标侧：预检那条带「没写过文件」",
			side: sideTarget, path: "/data/app",
			avail: 1 << 30, need: 3 << 30, beforeWrite: true,
			wantShort: true,
			wantMsg:   "sshfleet.transfer.target_space_short: /data/app: 1 GB available, 3 GB needed; no file written",
		},
		{
			name: "目标侧：失败后再查那条给剩余未传量、不说没写过文件",
			side: sideTarget, path: "/data/app",
			avail: 512 << 20, need: 1200 << 20, beforeWrite: false,
			wantShort: true,
			wantMsg:   "sshfleet.transfer.target_space_short: /data/app: 512 MB available, 1.17 GB left to send",
		},
		{
			name: "暂存侧：路径固定是 /tmp",
			side: sideTmp, path: "/tmp",
			avail: 48 << 20, need: 320 << 20, beforeWrite: true,
			wantShort: true,
			wantMsg:   "sshfleet.transfer.tmp_space_short: /tmp: 48 MB available, 320 MB needed; no file written",
		},
		{
			name: "暂存侧：够用时什么都不发",
			side: sideTmp, path: "/tmp",
			avail: 4 << 30, need: 64 << 20, beforeWrite: true,
			wantShort: false, wantMsg: "",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			msg, short := judgeSpace(c.side, c.path, c.avail, c.need, c.beforeWrite)
			if short != c.wantShort {
				t.Fatalf("判满 = %v，期望 %v", short, c.wantShort)
			}
			if msg != c.wantMsg {
				t.Fatalf("报文 =\n  %q\n期望\n  %q", msg, c.wantMsg)
			}
		})
	}
}

func TestAvailBytes(t *testing.T) {
	// 100 个空块、80 个非 root 可用块，每块 1KB；Bsize 故意写成另一个数，
	// 用来锁住「按 Frsize 算」这件事（Bavail/Bfree 都以 Frsize 为单位）。
	st := &sftp.StatVFS{Bsize: 4096, Frsize: 1024, Blocks: 200, Bfree: 100, Bavail: 80}

	if got := availBytes(st, true); got != 102400 {
		t.Fatalf("root 可用 = %d，期望 102400（用 Bfree）", got)
	}
	if got := availBytes(st, false); got != 81920 {
		t.Fatalf("非 root 可用 = %d，期望 81920（用 Bavail）", got)
	}

	// Frsize 缺失时退回 Bsize：有些实现只填 Bsize。
	st.Frsize = 0
	if got := availBytes(st, false); got != 81920*4 {
		t.Fatalf("Frsize 缺失时 = %d，期望 %d（退回 Bsize）", got, 81920*4)
	}
}

func TestHumanBytes(t *testing.T) {
	cases := map[int64]string{
		0:              "0 B",
		1536:           "1.5 KB",
		1 << 20:        "1 MB",
		48 << 20:       "48 MB",
		1200 << 20:     "1.17 GB",
		2 << 30:        "2 GB",
		3422552064:     "3.19 GB",
		3221225471 + 1: "3 GB",
	}
	for in, want := range cases {
		if got := humanBytes(in); got != want {
			t.Fatalf("humanBytes(%d) = %q，期望 %q", in, got, want)
		}
	}
}
