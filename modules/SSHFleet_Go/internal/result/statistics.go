// 结果统计（对位旧 statistics.py）：成功/失败计数、总数校验、失败分类排序、
// 按分类收集 IP、成功分类按模式确定、全局耗时。
//
// 成败与分类读的是结果判定（internal/verdict）写好的结论，本包不再自行判定。
package result

import (
	"sort"
	"strconv"
	"strings"
	"time"

	"sshfleet/internal/cli"
	"sshfleet/internal/nodelist"
	"sshfleet/internal/ssh"
	"sshfleet/internal/verdict"
)

// CategoryCount 分类及其数量。
type CategoryCount struct {
	Category string
	Count    int
}

// Stats 统计结果。
type Stats struct {
	ResultsTotal         int
	NodesTotal           int
	NotExecuted          int    // 节点总数 − 结果数：中断后没跑完（结果被丢弃）的台数
	Interrupted          bool   // 本次执行由中断信号结束
	Verify               string // 结果数与节点数一致时「通过」，不一致时「异常」；中断导致的缺额置空（不共用「异常」）
	SuccessCounts        int
	FailCounts           int
	ConnectSuccessCounts int
	ConnectFailCounts    int
	SortedFailCategories []CategoryCount
	SuccessCategory      string
	SuccessIPsCount      int
	SortedSuccessIPs     []string
	CategoryIPMap        map[string][]string
	GlobalStartTime      time.Time
	GlobalStopTime       time.Time
	GlobalCostTime       float64
}

// Statistics 计算结果统计信息。items 是判定完毕的结果切片（判定在 batch 的
// worker 协程里已做完，这里只读结论）。
func Statistics(items []ssh.Result, nodes *nodelist.Nodes, a *cli.Args, start, stop time.Time, interrupted bool) *Stats {
	// 成功分类由模式名字表给（命令/脚本→执行成功、上传/下载→传输成功、改密→改密成功）。
	// 「算不算传输类」从此只有名字表一处判据，统计侧不再自己判。
	successCategory := a.ModeName().Info().SuccessCategory

	stats := &Stats{
		ResultsTotal:    len(items),
		NodesTotal:      nodes.Len(),
		NotExecuted:     nodes.Len() - len(items),
		Interrupted:     interrupted,
		CategoryIPMap:   map[string][]string{},
		SuccessCategory: successCategory,
		GlobalStartTime: start,
		GlobalStopTime:  stop,
		GlobalCostTime:  stop.Sub(start).Seconds(),
	}
	// 缺额分两种：中断没跑完（用户自己按的，不该叫「异常」），与其它原因的不一致（真异常）。
	switch {
	case stats.NotExecuted == 0:
		stats.Verify = "通过"
	case interrupted:
		stats.Verify = ""
	default:
		stats.Verify = "异常"
	}

	counts := map[string]int{}
	for _, r := range items {
		// 连接成败也在这里一并算：主干收尾行的四个数都读统计，各处不再自己数一遍。
		if r.ConnectSuccess {
			stats.ConnectSuccessCounts++
		} else {
			stats.ConnectFailCounts++
		}
		category := r.Category
		if r.Verdict == verdict.Success {
			stats.SuccessCounts++
			category = successCategory
		} else {
			stats.FailCounts++
		}

		counts[category]++
		stats.CategoryIPMap[category] = append(stats.CategoryIPMap[category], r.IP)
	}

	delete(counts, successCategory)
	for category, count := range counts {
		stats.SortedFailCategories = append(stats.SortedFailCategories, CategoryCount{category, count})
	}
	sort.SliceStable(stats.SortedFailCategories, func(i, j int) bool {
		return stats.SortedFailCategories[i].Count > stats.SortedFailCategories[j].Count
	})

	successIPs := stats.CategoryIPMap[successCategory]
	delete(stats.CategoryIPMap, successCategory)
	stats.SuccessIPsCount = len(successIPs)
	stats.SortedSuccessIPs = SortIPs(successIPs)
	for category, ips := range stats.CategoryIPMap {
		stats.CategoryIPMap[category] = SortIPs(ips)
	}
	return stats
}

// SortIPs IP 数值序排序（对位旧 sort_ips：按各段数值比较，非法段按字符串兜底）。
func SortIPs(ips []string) []string {
	out := append([]string(nil), ips...)
	sort.SliceStable(out, func(i, j int) bool { return ipLess(out[i], out[j]) })
	return out
}

func ipLess(a, b string) bool {
	as, bs := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < len(as) && i < len(bs); i++ {
		ai, aerr := strconv.Atoi(as[i])
		bi, berr := strconv.Atoi(bs[i])
		if aerr == nil && berr == nil {
			if ai != bi {
				return ai < bi
			}
			continue
		}
		if as[i] != bs[i] {
			return as[i] < bs[i]
		}
	}
	return len(as) < len(bs)
}
