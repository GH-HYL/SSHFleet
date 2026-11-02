package parse

import "time"

// 解析站读时钟即报：它要时间就从纸上拿，不该自己取（这是本判据的违规样本）。
func Run() time.Time {
	return time.Now()
}
