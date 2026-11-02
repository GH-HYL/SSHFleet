package contract

import "time"

// 第三张纸：结算交报告（口径：总共怎么样）。
//
// 它只装下一站要用的：报告站要印屏幕统计块与 `report.txt`，两样都吃这张纸。
// 逐台明细跟着走，因为报告要贴原文；附加格原样传来，结算一个字段都没打开过。
//
// 判据见 ADR-0035 决定一、上一张图 15 号工单第三节。

// CategoryCount 是一个分类与它的台数。
type CategoryCount struct {
	Category string
	Count    int

	// ExitCode 是该分类携带的退出码。只有「执行失败(退出码N)」这类分类有值，其余为 nil。
	// 它同时内嵌在分类名里，但那是给人看的字：要这个数就读这里，不要去解名字。
	ExitCode *int
}

// CategoryIPs 是一个分类下面那些机器的 IP，按老版的次序排好。
type CategoryIPs struct {
	Category string
	IPs      []string
}

// Ledger 是总账：报告站写屏幕统计块与 `report.txt` 第二段要的一切。
type Ledger struct {
	// —— 台数 ——
	Total       int // 这次打算做多少台（来自计划）
	Done        int // 真跑完了多少台（结果行数）
	NotRun      int // 没跑的：Total − Done。中断那笔差额也记在这里
	Interrupted bool

	// Verify 是「跑没跑齐」的结论：结果数与节点数一致时是「通过」，不一致是「异常」。
	// 中断造成的缺额置空——那不是异常，是没跑（老版口径，08 号工单的更新记录）。
	Verify string

	// —— 成败：只在这里产生 ——
	SuccessCounts int
	FailCounts    int

	// —— 连接 ——
	ConnectSuccessCounts int
	ConnectFailCounts    int

	// —— 分类分布 ——
	SuccessCategory string   // 本模式的成功分类名（命令与脚本是「执行成功」）
	SuccessIPs      []string // 成功那些机器
	FailCategories  []CategoryCount
	CategoryIPs     []CategoryIPs // 每个分类下面各有哪些 IP（含成功那一类；次序与分类次序一致）

	// —— 时间 ——
	StartedAt time.Time
	StoppedAt time.Time
	CostTime  float64 // 总耗时（秒），报告里印两位小数

	// —— 跟着走的 ——
	Items     []NodeResult // 逐台明细：报告要贴原文
	Extra     Extra        // 附加格：结算一个字段都没打开
	Tables    Tables
	RunMeta   RunMeta
	Resources Resources
}
