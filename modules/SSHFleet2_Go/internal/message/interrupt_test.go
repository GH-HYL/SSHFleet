package message

import "testing"

// 两句中断的话逐字钉住：屏幕那句与日志那句各是各的，谁也不许顺手改一个字。
func TestInterruptLines(t *testing.T) {
	if got := InterruptLine().Text(); got != "已收到中断信号，SSHFleet 停止执行（已完成节点的结果已写入日志/输出文件）\n" {
		t.Fatalf("屏幕那句不对：%q", got)
	}
	if got := InterruptLog().Text(); got != "收到中断信号，执行已停止" {
		t.Fatalf("日志那句不对：%q", got)
	}
}
