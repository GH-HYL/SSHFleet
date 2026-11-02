package settle

// 结算站：把第二张纸（每台的事实）解释成第三张纸（总共怎么样）。
//
// **成败只在这里解释**：分类等于本模式的成功分类 ⇒ 成功，否则失败（ADR-0035 决定四）。
// 有的模式压根没有退出码（上传、下载、改密），拿退出码猜成败会猜错，所以判据只有这一条。
//
// 要依赖全体的判断都等在这里做——只依赖一台的归类留在执行站。

import (
	"sort"
	"strconv"
	"strings"

	"sshfleet2/internal/contract"
	"sshfleet2/internal/message"
)

// 跑没跑齐那两个取值。中断造成的缺额不叫「异常」——那是使用者自己按的。
const (
	verifyPassed = "通过"
	verifyFailed = "异常"
)

// Run 递一张第二张纸，交回第三张纸。纯计算：不写终端、不写文件。
//
// 附加格一个字段都不打开，原样递给报告站（ADR-0035 决定九）。
func Run(paper contract.ResultPaper) contract.Ledger {
	mode := paper.Extra.Mode
	successCategory := message.SuccessCategory(mode)

	ledger := contract.Ledger{
		Total:           paper.Total,
		Done:            len(paper.Items),
		NotRun:          paper.Total - len(paper.Items),
		Interrupted:     paper.Interrupted,
		SuccessCategory: successCategory,
		StartedAt:       paper.StartedAt,
		StoppedAt:       paper.StoppedAt,
		CostTime:        paper.StoppedAt.Sub(paper.StartedAt).Seconds(),

		// 原样往下传：结算不看这两样，报告要看。
		Items:     paper.Items,
		Extra:     paper.Extra,
		Tables:    paper.Tables,
		RunMeta:   paper.RunMeta,
		Resources: paper.Resources,
	}

	// 缺额分两种：中断没跑完（用户自己按的，不该叫「异常」），与其它原因的不一致（真异常）。
	switch {
	case ledger.NotRun == 0:
		ledger.Verify = verifyPassed
	case paper.Interrupted:
		ledger.Verify = ""
	default:
		ledger.Verify = verifyFailed
	}

	counts := map[string]int{}
	ips := map[string][]string{}
	exitCodes := map[string]*int{}
	var order []string // 分类**首次出现**的先后

	for _, item := range paper.Items {
		if item.Connected {
			ledger.ConnectSuccessCounts++
		} else {
			ledger.ConnectFailCounts++
		}

		category := item.Category
		if message.IsSuccess(item, mode) {
			ledger.SuccessCounts++
			category = successCategory
		} else {
			ledger.FailCounts++
		}

		if _, seen := counts[category]; !seen {
			order = append(order, category)
		}
		counts[category]++
		// 「执行失败(退出码N)」这一类带退出码：从纸上搬一格出来，呈现侧就不必去解分类名。
		// 同一分类下各台的退出码必然相同（名字里就写着它），取第一次遇到的即可。
		if _, seen := exitCodes[category]; !seen && message.IsExitCodeCategory(category) {
			exitCodes[category] = item.ExitCode
		}
		ips[category] = append(ips[category], item.IP)
	}

	// 失败分类按数量降序；数量相同时按首现序（稳定排序接住 order 里的先后）。
	for _, category := range order {
		if category == successCategory {
			continue
		}
		ledger.FailCategories = append(ledger.FailCategories, contract.CategoryCount{
			Category: category,
			Count:    counts[category],
			ExitCode: exitCodes[category],
		})
	}
	sort.SliceStable(ledger.FailCategories, func(i, j int) bool {
		return ledger.FailCategories[i].Count > ledger.FailCategories[j].Count
	})

	// 逐类 IP：失败分类按上面的次序，成功那一类排在最后（统计块里它也在最后）。
	ledger.SuccessIPs = sortIPs(ips[successCategory])
	for _, c := range ledger.FailCategories {
		ledger.CategoryIPs = append(ledger.CategoryIPs, contract.CategoryIPs{
			Category: c.Category,
			IPs:      sortIPs(ips[c.Category]),
		})
	}
	if successCategory != "" {
		ledger.CategoryIPs = append(ledger.CategoryIPs, contract.CategoryIPs{
			Category: successCategory,
			IPs:      sortIPs(ips[successCategory]),
		})
	}

	return ledger
}

// sortIPs IP 数值序（对位老版 `SortIPs`）：按各段数值比，非法段按字符串兜底。
//
// 返回新切片：调用方拿到的是自己的副本，改它影响不到纸上的原文。
func sortIPs(list []string) []string {
	out := append([]string(nil), list...)
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
