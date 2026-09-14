// 结果统计（对位旧 statistics.py）：成功/失败计数、总数校验、失败分类排序、
// 按分类收集 IP、成功分类按模式确定、全局耗时。
package result

import (
	"sort"
	"strconv"
	"strings"
	"time"

	"sshfleet/internal/batch"
	"sshfleet/internal/cli"
	"sshfleet/internal/nodelist"
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
	Verify               string // 结果数与节点数一致时「通过」，否则「异常」
	SuccessCounts        int
	FailCounts           int
	SortedFailCategories []CategoryCount
	SuccessCategory      string
	SuccessIPsCount      int
	SortedSuccessIPs     []string
	CategoryIPMap        map[string][]string
	GlobalStartTime      time.Time
	GlobalStopTime       time.Time
	GlobalCostTime       float64
}

// ModeOf 由命令行参数确定执行模式（execute / upload / download）。
func ModeOf(a *cli.Args) string {
	switch {
	case a.Upload != "":
		return "upload"
	case a.Download != "":
		return "download"
	default:
		return "execute"
	}
}

// Statistics 计算结果统计信息。
func Statistics(results *batch.Results, nodes *nodelist.Nodes, a *cli.Args, kw *Keywords, start, stop time.Time) *Stats {
	mode := ModeOf(a)
	successCategory := SuccessCategoryExecute
	if mode != "execute" {
		successCategory = SuccessCategoryTransport
	}

	stats := &Stats{
		ResultsTotal:    results.Len(),
		NodesTotal:      nodes.Len(),
		CategoryIPMap:   map[string][]string{},
		SuccessCategory: successCategory,
		GlobalStartTime: start,
		GlobalStopTime:  stop,
		GlobalCostTime:  stop.Sub(start).Seconds(),
	}
	if stats.NodesTotal == stats.ResultsTotal {
		stats.Verify = "通过"
	} else {
		stats.Verify = "异常"
	}

	counts := map[string]int{}
	for _, r := range results.Items {
		exitBool := r.ExitCode != nil && *r.ExitCode == 0
		if exitBool {
			stats.SuccessCounts++
		} else {
			stats.FailCounts++
		}

		category := Classify(Case{
			ExitCode:     r.ExitCode,
			Error:        strOf(r.Error),
			Output:       r.Output,
			AuthFailure:  strOf(r.AuthFailure),
			Mode:         mode,
			SuccessFiles: r.SuccessFiles,
			FailedFiles:  r.FailedFiles,
		}, kw)

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

func strOf(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
