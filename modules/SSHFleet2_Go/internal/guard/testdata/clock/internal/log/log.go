package log

import "time"

// 日志包：每行那个时间戳与轮转改名都要「现在」，这一处许读。
func stamp() string {
	return time.Now().Format("2006-01-02 15:04:05.000")
}
