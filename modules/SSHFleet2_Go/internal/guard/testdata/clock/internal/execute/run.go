package execute

import "time"

// 执行站：耗时、超时、握手 deadline 全是实时的，这一处许读。
func Run() time.Duration {
	start := time.Now()
	return time.Since(start)
}
