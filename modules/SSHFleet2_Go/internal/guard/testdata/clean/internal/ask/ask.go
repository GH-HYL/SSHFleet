package ask

import (
	"fixture/internal/console"
	"fixture/internal/message"
)

// Ask 问一句。叶子之间的例外只有 ask → console 与 ask → message 两条。
func Ask() string {
	console.Line()
	return message.Say()
}
