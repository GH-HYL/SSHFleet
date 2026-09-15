package output

import (
	"bytes"
	"fmt"
	"sort"
	"strings"
	"testing"

	"sshfleet/internal/batch"
)

// 进度块的重绘位置回归。
//
// 这类 bug 只在真终端上肉眼可见（进度块整体下移、上方堆空行），用普通字符串断言
// 抓不住，故此处用一个极简终端模型：只跟踪光标行与各物理行的可见文本，
// 认 CSI A（上移）/ CSI B（下移）/ CSI K（清行）/ \r / \n。
// 不模拟滚动——被测逻辑依赖的是相对行位移，终端滚动不改变相对关系。

type ansiTerm struct {
	row  int
	col  int
	rows map[int][]rune
}

func newAnsiTerm() *ansiTerm { return &ansiTerm{rows: map[int][]rune{}} }

func drain(buf *bytes.Buffer) string {
	s := buf.String()
	buf.Reset()
	return s
}

// put 从当前列写入一段可见文本（同一行会被多段颜色转义切成多段，必须按列拼接而非覆盖）。
func (t *ansiTerm) put(text string) {
	line := t.rows[t.row]
	for len(line) < t.col {
		line = append(line, ' ')
	}
	for i, r := range []rune(text) {
		if pos := t.col + i; pos < len(line) {
			line[pos] = r
		} else {
			line = append(line, r)
		}
	}
	t.rows[t.row] = line
	t.col += len([]rune(text))
}

func (t *ansiTerm) feed(s string) {
	for i := 0; i < len(s); {
		switch {
		case s[i] == 0x1b && i+1 < len(s) && s[i+1] == '[':
			j := i + 2
			n := 0
			for j < len(s) && s[j] >= '0' && s[j] <= '9' {
				n = n*10 + int(s[j]-'0')
				j++
			}
			if j >= len(s) {
				return
			}
			switch s[j] {
			case 'A':
				if t.row -= n; t.row < 0 {
					t.row = 0
				}
			case 'B':
				t.row += n
			case 'C':
				t.col += n
			case 'D':
				if t.col -= n; t.col < 0 {
					t.col = 0
				}
			case 'K', 'J': // 擦到行尾：按当前列截断
				if line := t.rows[t.row]; len(line) > t.col {
					t.rows[t.row] = line[:t.col]
				}
			}
			i = j + 1
		case s[i] == '\r':
			t.col = 0
			i++
		case s[i] == '\n':
			t.row++
			t.col = 0
			i++
		default:
			j := i
			for j < len(s) && s[j] != 0x1b && s[j] != '\r' && s[j] != '\n' {
				j++
			}
			t.put(s[i:j])
			i = j
		}
	}
}

// line 某行的可见文本。
func (t *ansiTerm) line(row int) string { return string(t.rows[row]) }

// dump 按行号顺序导出可见内容，供断言失败时定位。
func (t *ansiTerm) dump() string {
	idx := make([]int, 0, len(t.rows))
	for r := range t.rows {
		idx = append(idx, r)
	}
	sort.Ints(idx)
	var b strings.Builder
	for _, r := range idx {
		fmt.Fprintf(&b, "%3d|%s\n", r, string(t.rows[r]))
	}
	return b.String()
}

// blockTop 进度块首行（总进度行）所在行号；找不到返回 -1。
func (t *ansiTerm) blockTop(label string) int {
	for r, line := range t.rows {
		if strings.Contains(string(line), label) {
			return r
		}
	}
	return -1
}

// uploadSnapshot active 个「已开始传输」的节点（节点条只在已有字节时授予）。
func uploadSnapshot(active int) batch.Snapshot {
	s := batch.Snapshot{Total: 30, BytesDone: 400, BytesTotal: 1000}
	for i := 0; i < active; i++ {
		s.Nodes = append(s.Nodes, batch.NodeSnapshot{
			Seq: i, IP: fmt.Sprintf("10.0.0.%d", i+1),
			Bytes: 40, TotalBytes: 100, TotalFiles: 2,
		})
	}
	return s
}

// 进度块锚点：同高重绘不下移，结果输出只把块往下推「输出行数」那么多，中间不留空行。
func TestTransferProgressBlockStaysAnchored(t *testing.T) {
	var buf bytes.Buffer
	ui := NewProgressUI(&buf, "upload", 30)
	term := newAnsiTerm()
	term.feed(drain(&buf))

	snap := uploadSnapshot(3)
	ui.Update(snap)
	term.feed(drain(&buf))

	top := term.blockTop("上传进度")
	if top < 0 {
		t.Fatalf("首帧未画出进度块，终端内容：\n%s", term.dump())
	}

	// 模拟 20 次结果输出（每次一行）+ 每次跟随 3 帧同高重绘
	expectTop := top
	for round := 1; round <= 20; round++ {
		ui.PrintAbove(fmt.Sprintf("【10.0.0.%d】 连接: 成功 - 0.010s", round))
		term.feed(drain(&buf))
		expectTop++
		if got := term.blockTop("上传进度"); got != expectTop {
			t.Fatalf("第 %d 轮结果输出后进度块应在第 %d 行，实际第 %d 行", round, expectTop, got)
		}

		for frame := 0; frame < 3; frame++ {
			ui.Update(snap)
			term.feed(drain(&buf))
			if got := term.blockTop("上传进度"); got != expectTop {
				t.Fatalf("第 %d 轮第 %d 帧同高重绘后进度块应在第 %d 行，实际第 %d 行（块整体下移）",
					round, frame+1, expectTop, got)
			}
		}

		if line := term.line(expectTop - 1); strings.TrimSpace(line) == "" {
			t.Fatalf("第 %d 轮后进度块上方（第 %d 行）出现空行：块被推离了结果输出", round, expectTop-1)
		}
	}
}

// 块收缩：节点条完成后旧行必须被擦掉，不能在块下方残留半截进度条
// （对应「统计结果叠在半截进度条中间」那次的修复）。
func TestTransferProgressBlockClearsRowsAfterShrink(t *testing.T) {
	var buf bytes.Buffer
	ui := NewProgressUI(&buf, "upload", 30)
	term := newAnsiTerm()
	term.feed(drain(&buf))

	ui.Update(uploadSnapshot(20))
	term.feed(drain(&buf))

	// 全部完成：节点条释放，块收缩回 3 行
	ui.Update(batch.Snapshot{Total: 30, Completed: 30, Succeeded: 30, BytesDone: 1000, BytesTotal: 1000})
	term.feed(drain(&buf))

	top := term.blockTop("上传进度")
	if top < 0 {
		t.Fatalf("收缩后未画出进度块，终端内容：\n%s", term.dump())
	}
	for r := range term.rows {
		if line := term.line(r); r > top+2 && strings.Contains(line, "━") {
			t.Fatalf("块收缩后第 %d 行仍残留旧的节点条：%q", r, line)
		}
	}
}

// 单行块（命令模式）也必须停回块起点：否则结果明细会与进度条互相顶走。
func TestCommandProgressBlockStaysAnchored(t *testing.T) {
	var buf bytes.Buffer
	ui := NewProgressUI(&buf, "execute", 30)
	term := newAnsiTerm()
	term.feed(drain(&buf))

	ui.Update(batch.Snapshot{Total: 30, Completed: 5, Succeeded: 5})
	term.feed(drain(&buf))
	top := term.blockTop("执行进度")
	if top < 0 {
		t.Fatalf("首帧未画出进度条，终端内容：\n%s", term.dump())
	}

	expectTop := top
	for round := 1; round <= 5; round++ {
		ui.PrintAbove("【10.0.0.9】 分类: 执行成功")
		term.feed(drain(&buf))
		expectTop++
		if got := term.blockTop("执行进度"); got != expectTop {
			t.Fatalf("第 %d 轮结果输出后进度条应在第 %d 行，实际第 %d 行", round, expectTop, got)
		}
		if line := term.line(expectTop - 1); strings.TrimSpace(line) == "" {
			t.Fatalf("第 %d 轮后进度条上方（第 %d 行）出现空行", round, expectTop-1)
		}
	}
}
