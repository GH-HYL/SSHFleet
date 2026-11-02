package execute

import "fixture/internal/console"

// Run 不犯规：执行站是唯一许拿这条流的包（进度界面要独占屏幕）。
func Run() {
	_ = console.Writer()
}
