// Package diagnose 一次性收齐「终端看不到命令输出」的取证信息。
//
// 为什么要有这个包：目标环境（Windows + iBMC）这边碰不到，测试机会只有一次，
// 而且是别人替我们跑——他只会换一个可执行文件再执行一次，然后请他把日志发回来。
// 所以这一次必须把「判定根因要用到的每一个事实」都记下来，不能靠事后追问。
//
// 记什么、为什么（按判定时的用途排）：
//   - 远端原始输出的字节统计（CR / LF / ESC / NUL / 控制字符各几个、每行行尾是什么）
//     —— 决定「是不是 CR 在捣乱」的直接证据；
//   - 远端原始输出的十六进制与转义形式（前若干字节）—— 肉眼能复核，不必会看二进制；
//   - 会话是否申请了 tty、本地终端是否支持 ANSI —— 决定终端会怎么解释这段字节；
//   - 最终交给终端的那段文本的字节构成 —— 与上面两者对照，确认「送到终端的到底是什么」。
//
// 本包只读事实、只写日志，不改任何执行行为。记录器见 recorder.go。
package diagnose

import (
	"fmt"
	"os"
	"strings"
	"unicode/utf8"

	"github.com/mattn/go-isatty"
)

// ByteStats 一段远端输出里各类字节的计数。导出：它是本包的对外判据，测试与调用方据此读事实。
type ByteStats struct {
	Total     int
	CR        int
	LF        int
	ESC       int
	NUL       int
	OtherCtrl int // 除 CR / LF / ESC / NUL 之外的控制字符（\b、\v、\f、DEL…）
	ValidUTF8 bool
}

// LineEndings 每行行尾的形态统计。导出理由同 ByteStats。
type LineEndings struct {
	CRLF int
	LF   int
	CR   int
	None int
}

func (s ByteStats) String() string {
	return fmt.Sprintf("共 %d 字节：CR=%d LF=%d ESC=%d NUL=%d 其它控制字符=%d 合法UTF-8=%v",
		s.Total, s.CR, s.LF, s.ESC, s.NUL, s.OtherCtrl, s.ValidUTF8)
}

func (e LineEndings) String() string {
	return fmt.Sprintf("CRLF=%d 只LF=%d 只CR=%d 无换行=%d", e.CRLF, e.LF, e.CR, e.None)
}

// Stats 统计一段文本的字节构成。
func Stats(s string) ByteStats {
	st := ByteStats{Total: len(s), ValidUTF8: utf8.ValidString(s)}
	for i := 0; i < len(s); i++ {
		switch b := s[i]; {
		case b == '\r':
			st.CR++
		case b == '\n':
			st.LF++
		case b == 0x1b:
			st.ESC++
		case b == 0x00:
			st.NUL++
		case b < 0x20 || b == 0x7f:
			st.OtherCtrl++
		}
	}
	return st
}

// Endings 统计行尾形态。判定规则：`\r\n` 算一个 CRLF；孤立的 `\r` 算只 CR；
// 其余 `\n` 算只 LF；末段不为空则记一次「无换行」。
func Endings(s string) LineEndings {
	var e LineEndings
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '\r':
			if i+1 < len(s) && s[i+1] == '\n' {
				e.CRLF++
				i++
			} else {
				e.CR++
			}
		case '\n':
			e.LF++
		}
	}
	if s != "" && !strings.HasSuffix(s, "\n") && !strings.HasSuffix(s, "\r") {
		e.None++
	}
	return e
}

// HexDump 前 limit 个字节的十六进制（每行 16 字节，右侧附可打印字符）。
// 只给前若干字节：日志要能直接看，过长会淹没其它信息。
func HexDump(s string, limit int) string {
	b := []byte(s)
	if len(b) > limit {
		b = b[:limit]
	}
	var sb strings.Builder
	for off := 0; off < len(b); off += 16 {
		end := off + 16
		if end > len(b) {
			end = len(b)
		}
		fmt.Fprintf(&sb, "    %08x  ", off)
		for i := off; i < end; i++ {
			fmt.Fprintf(&sb, "%02x ", b[i])
		}
		for i := end; i < off+16; i++ {
			sb.WriteString("   ")
		}
		sb.WriteString(" |")
		for i := off; i < end; i++ {
			if b[i] >= 0x20 && b[i] < 0x7f {
				sb.WriteByte(b[i])
			} else {
				sb.WriteByte('.')
			}
		}
		sb.WriteString("|\n")
	}
	if len([]byte(s)) > limit {
		fmt.Fprintf(&sb, "    …（以下省略 %d 字节，完整内容见 terminal-output.txt）\n", len([]byte(s))-limit)
	}
	return strings.TrimRight(sb.String(), "\n")
}

// Escaped 把控制字符写成可见形式（\\r、\\n、\\x1b 等），供日志里一眼看懂。
// 中文等普通字符原样保留——目标机回的多是英文，保留可读性比纯 ASCII 更有用。
func Escaped(s string, limit int) string {
	var sb strings.Builder
	n := 0
	for _, r := range s {
		if n >= limit {
			sb.WriteString("…")
			break
		}
		n++
		switch r {
		case '\r':
			sb.WriteString(`\r`)
		case '\n':
			sb.WriteString(`\n`)
		case '\t':
			sb.WriteString(`\t`)
		case 0x1b:
			sb.WriteString(`\x1b`)
		case 0x00:
			sb.WriteString(`\x00`)
		case '\b':
			sb.WriteString(`\b`)
		case '\v':
			sb.WriteString(`\v`)
		case '\f':
			sb.WriteString(`\f`)
		default:
			if r < 0x20 || r == 0x7f {
				fmt.Fprintf(&sb, `\x%02x`, r)
				continue
			}
			sb.WriteRune(r)
		}
	}
	return sb.String()
}

// LocalEnv 本地运行环境的事实：stdout 是不是终端、TERM、颜色能力、Windows 控制台 VT。
//
// 只收事实，不下结论——读日志的人拿它对照远端字节，判断「终端会怎么解释这段输出」。
func LocalEnv(colorEnabled bool, consoleVT bool, termName string) string {
	return fmt.Sprintf("stdout是终端=%v，TERM=%q，颜色序列=%v，Windows控制台VT=%v",
		isTerminal(), termName, colorEnabled, consoleVT)
}

// isTerminal 本进程 stdout 是否连在终端上（重定向到文件时为否）。
//
// 用 isatty 而不是 Stat 的字符设备位：Windows 上 cygwin/mintty 的管道也算终端，
// 与呈现层（internal/output）判断「要不要启动进度界面」用的是同一个库、同一口径——
// 两处口径一旦分叉，「日志说 stdout 是终端，可进度界面没起来」这种矛盾就会出现在取证记录里。
func isTerminal() bool {
	fd := os.Stdout.Fd()
	return isatty.IsTerminal(fd) || isatty.IsCygwinTerminal(fd)
}

// quote 给一段可能含空格的文本加上直引号：日志里空值要看得见（"" 与「什么都没有」不同）。
func quote(s string) string { return `"` + s + `"` }

// splitLines 按行拆分（只用于把十六进制转储逐行写进日志）。
func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

// joinComma 用顿号连接若干片段。
func joinComma(items []string) string {
	return strings.Join(items, "、")
}
