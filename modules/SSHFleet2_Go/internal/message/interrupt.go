package message

// InterruptLine 是执行期被中断时屏幕补的那一句（老版 `fmt.Println` 的原话，无颜色）。
//
// 次序由塔尖定：结算算完 → 执行期日志收尾行 → 这一句 → 统计块。字只在这里成形。
func InterruptLine() Line {
	return Line{s: "已收到中断信号，SSHFleet 停止执行（已完成节点的结果已写入日志/输出文件）\n"}
}

// InterruptLog 是同一件事写进工具日志的那一句（老版 `logger.Warn` 的原话）。
func InterruptLog() Line {
	return Line{s: "收到中断信号，执行已停止"}
}
