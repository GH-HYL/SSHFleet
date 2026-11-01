package console

import (
	"os"

	"sshfleet2/internal/contract"
)

// Print 把话写到标准输出——全工程唯一的终端出口（ADR-0032 第四条）。
//
// 它只收 contract.Text：字由 message 造（那里是唯一拼句处），这里一个字都不改。
// 契约包不许有函数，所以 Text 的字段是导出的——拦的是顺手不是存心，与那格密封件同一口径。
func Print(t contract.Text) {
	_, _ = os.Stdout.WriteString(t.S)
}
