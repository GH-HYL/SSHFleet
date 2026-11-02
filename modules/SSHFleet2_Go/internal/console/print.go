package console

import (
	"os"

	"sshfleet2/internal/message"
)

// Print 把话写到终端——全工程唯一的终端出口（ADR-0032 第四条）。
//
// 它只收 `message.Line`：那个类型的字段不导出，别的包拼不出来，也就没有东西可塞进来。
// 于是「自己取个数拼一句直接打出去」在编译期就过不去，不用靠检查盯。
//
// 两条流按那句话自己的标记走：错误走 stderr、其余走 stdout。老版里同一类失败走哪条流是
// 固定的（错误套 [ERROR] 走 stderr，危险框与「操作已取消」走 stdout），重定向时看得见，
// 所以流跟着句子走，不由塔尖猜。
//
// 颜色也在这里定去留：message 拼出来的句子一律带色码，这里按老版 detectColor 的判据
// 决定留着还是剥掉。判据只有一份，就在本包。
func Print(l message.Line) {
	w := os.Stdout
	if l.Stderr() {
		w = os.Stderr
	}
	s := l.Text()
	if !colorSupported() {
		s = stripANSI(s)
	}
	_, _ = w.WriteString(s)
}

// stripANSI 去掉 ANSI 转义序列（CSI 形态：ESC [ … 终止字节）。
//
// 只在「控制台连 VT 都开不起来」那一档才走到这里——那是老版 ANSI() 返回空串的同一种情形。
func stripANSI(s string) string {
	var out []byte
	for i := 0; i < len(s); i++ {
		if s[i] == 0x1b && i+1 < len(s) && s[i+1] == '[' {
			j := i + 2
			for j < len(s) && (s[j] < 0x40 || s[j] > 0x7e) {
				j++
			}
			if j < len(s) {
				i = j
				continue
			}
		}
		out = append(out, s[i])
	}
	return string(out)
}

// Prepare 在打第一屏之前调一次：把 Windows 控制台的输出码页摆正。
// 非 Windows 上是空动作。
func Prepare() { adaptCodepage() }

// Restore 在退出之前调一次：把码页还回去。
//
// 幂等——正常退出、致命退出、信号兜底三条路可能都调它。os.Exit 不跑 defer，
// 所以每一条退出路径都得自己记得调；调重了没事，漏了就是整窗乱码。
func Restore() { restoreCodepage() }
