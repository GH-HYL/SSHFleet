package ssh

import (
	"strings"
	"testing"

	"github.com/pkg/sftp"
)

// 上传前判满的判据与报文（工单 01、02）。
//
// # 为什么这里测的是包内私有函数
//
// 《个人开发规范》「三·结构·3」写着 MUST NOT 测私有函数、不测内部实现。这一组是有意的例外，
// 由作者 2026-10-27 裁定保留，理由写在下面——**不是**「纯函数好测」这种方便话。
//
// 判据要防的是「重构后行为没变、测试却红了」。这里测的是**决定**，不是函数形状：
// 每个用例对应一条规格或 ADR 写死的判断，而它们改了就是行为改了，测试该红。具体是：
//
//   - TestJudgeSpace 的边界两条：硬比不留余量 —— 可用恰好等于需要不算满、少一个字节就算满。
//     真机上没法把一块卷的可用量卡到「恰好等于文件大小」，这条边界只能在这里验。
//   - TestAvailBytes：`--sudo` 下以 root 写要看 Bfree（含保留块）、否则看 Bavail，块大小按
//     Frsize 折算。这两处能安静错到离谱：拿 Bsize 当 Frsize 会差 4 倍；满盘前用 Bavail
//     判会在一块还有 5% 保留块的盘上报「已满」。真机要验第二种，得把盘填进「只剩 5%」那个
//     窗口里，做不稳。
//   - TestSpaceShortDegrade：某一侧探不到、两侧都探不到时不许挡住上传。真机上造不出
//     「服务端不支持这个扩展」——手边只有 OpenSSH，而它支持。少了这条，「探不到」这条
//     路径就是零证据，而它错了的后果是「本来能传的机器传不了了」。
//
// 它买不到的是**接线**——预检接在哪个位置、失败分支在回滚之前还是之后，这些由
// space_precheck_test.go 里的真机用例管，本文件一概不碰。
//
// # 什么时候可以推翻
//
// 这个例外绑在上面的场景上，场景不成立就该跟着撤，不必再问：
//
//   - 真机上能把边界卡准了（比如卷大小可以按字节精确控制到「刚好不够」）→ 删 TestJudgeSpace
//     的边界两条，挪进真机用例；
//   - 手边有了不支持 statvfs 的服务端样本 → 删 TestSpaceShortDegrade，用真机验降级；
//   - `availBytes` 的字段选择被挪到别处、或平台自己给出可用字节数 → 删 TestAvailBytes；
//   - `judgeSpace` / `availBytes` / `humanBytes` 被合并进别的函数，签名一变就要跟着改——
//     改一次是成本，改到第三次说明这层不该单独存在，那时连测试带函数一起并掉，别硬撑。
//
// 期望值一律写成手算的字面量，不拿被测代码再算一遍——否则按构造必然通过，
// 判满判反了也发现不了。

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
			name: "目标目录写成 /data/app/ 时，报文里不留尾斜杠",
			side: sideTarget, path: "/data/app/",
			avail: 1 << 30, need: 3 << 30, beforeWrite: true,
			wantShort: true,
			wantMsg:   "sshfleet.transfer.target_space_short: /data/app: 1 GB available, 3 GB needed; no file written",
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
	// 只留别处没覆盖的：进位到两位小数、以及 KB/B 两档。
	// 1GB / 2GB / 3GB / 48MB / 320MB / 512MB / 1.17GB 这些形态，TestJudgeSpace 的报文里
	// 已经逐字锁过了，这里不再铺一遍。
	cases := map[int64]string{
		0:          "0 B",
		1536:       "1.5 KB",
		1 << 20:    "1 MB",
		3422552064: "3.19 GB", // 3.1875 GB → 四舍五入到两位
	}
	for in, want := range cases {
		if got := humanBytes(in); got != want {
			t.Fatalf("humanBytes(%d) = %q，期望 %q", in, got, want)
		}
	}
}

// 降级是按侧算的：某一侧探不到，只少那一侧的判断；两侧都探不到，等于没有预检。
//
// 真机上造不出「探测失败」（手边只有 OpenSSH，它支持这个扩展），而这条恰恰最不能出错：
// 探不到不许挡住上传。所以用一个假的探测函数喂出三种「探不到」的组合。
func TestSpaceShortDegrade(t *testing.T) {
	const (
		gb   = int64(1) << 30
		tmpP = "/tmp"
		tgtP = "/data/app"
	)

	cases := []struct {
		name    string
		probe   probeFunc
		sudo    bool
		wantMsg string // 期望命中的报文名；空即「什么都不说」
	}{
		{
			name: "非 sudo：目标不够就说目标",
			probe: func(p string, _ bool) (int64, bool) {
				return gb, p == tgtP
			},
			sudo:    false,
			wantMsg: "sshfleet.transfer.target_space_short",
		},
		{
			name: "非 sudo：目标探不到就什么都不说（不阻断）",
			probe: func(string, bool) (int64, bool) {
				return 0, false
			},
			sudo:    false,
			wantMsg: "",
		},
		{
			name: "sudo：目标够、/tmp 不够，说 /tmp",
			probe: func(p string, _ bool) (int64, bool) {
				switch p {
				case tgtP:
					return 10 * gb, true
				case tmpP:
					return 1 << 20, true
				}
				return 0, false
			},
			sudo:    true,
			wantMsg: "sshfleet.transfer.tmp_space_short",
		},
		{
			name: "sudo：目标探不到，仍按 /tmp 判",
			probe: func(p string, _ bool) (int64, bool) {
				return 1 << 20, p == tmpP
			},
			sudo:    true,
			wantMsg: "sshfleet.transfer.tmp_space_short",
		},
		{
			name: "sudo：两侧都探不到，什么都不说",
			probe: func(string, bool) (int64, bool) {
				return 0, false
			},
			sudo:    true,
			wantMsg: "",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := spaceShort(c.probe, tgtP, c.sudo, 2*gb, 64<<20, true)
			if c.wantMsg == "" {
				if got != "" {
					t.Fatalf("不该说话，实为 %q", got)
				}
				return
			}
			if !strings.Contains(got, c.wantMsg) {
				t.Fatalf("报文 = %q，期望命中 %s", got, c.wantMsg)
			}
		})
	}
}
