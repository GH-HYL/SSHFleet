package parse

import "fixture/internal/console"

// Run 犯规：解析站取了 console 的 writer 直接往屏幕写字节。
func Run() {
	_ = console.Writer()
}
