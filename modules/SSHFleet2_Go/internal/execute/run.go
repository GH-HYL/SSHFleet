package execute

// 执行站的入口：收下闸门放行的那张纸，按纸上的并发跑完每一台，交回第二张纸。
//
// 本单只做建连与认证（老版步骤 8 的「建连认证」那一段）：连上了就把「连上」那一格置真，
// 连不上就把报文与服务端提示写进「错误原文」。跑命令、定分类、刷屏是后面几张单的事
// （10、11、12），这里给它们留的位置是 `visit` 里「连上之后」那一段。
//
// 出口白名单：这个包对外只露 `Run` 一个函数，纸的类型都在契约包里（ADR-0037 决定三）。

import (
	"context"
	"sync"
	"time"

	"sshfleet2/internal/contract"
)

// Run 跑一遍：每台一条事实，按纸上的节点顺序交回。
//
// context 取消时**不再开工**，已开工的那台跑完照常上交（老版 `batch/pool.go` 的口径：
// 取消不等于没执行，目标机可能已经被改过）；压根没开跑的那台不造行，只在总台数里以缺额出现。
//
// 第二个返回值是「起不来」：本单还没有哪一种前提缺失会走到这里（配置、清单、凭据都在上游
// 判过），留着是因为站与站之间递东西的形状就两种——一张纸，或者起不来（ADR-0034 决定三）。
func Run(ctx context.Context, p *contract.Plan) (contract.ResultPaper, *contract.CannotStart) {
	paper := contract.ResultPaper{
		Total:     len(p.Nodes),
		Extra:     contract.Extra{Mode: p.Mode},
		Tables:    p.Tables,
		RunMeta:   p.RunMeta,
		StartedAt: time.Now(),
	}
	if len(p.Nodes) == 0 {
		paper.StoppedAt = paper.StartedAt
		return paper, nil
	}

	limit := p.Run.Concurrency
	if limit <= 0 || limit > len(p.Nodes) {
		limit = len(p.Nodes) // 0 表示不填，按全部并行（老版同口径）
	}

	// 每台一格。worker 各写自己那一格，互不相干；保序靠下标，不靠收上来的先后。
	type slot struct {
		res    contract.NodeResult
		filled bool
	}
	slots := make([]slot, len(p.Nodes))

	todo := make(chan int, len(p.Nodes))
	var wg sync.WaitGroup
	for i := 0; i < limit; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case idx, ok := <-todo:
					if !ok {
						return
					}
					slots[idx] = slot{res: visit(ctx, p, idx), filled: true}
				}
			}
		}()
	}

	go func() {
		defer close(todo)
		for i := range p.Nodes {
			select {
			case todo <- i:
			case <-ctx.Done():
				return
			}
		}
	}()

	wg.Wait()

	items := make([]contract.NodeResult, 0, len(slots))
	for _, s := range slots {
		if s.filled {
			items = append(items, s.res)
		}
	}
	paper.Items = items
	paper.StoppedAt = time.Now()
	return paper, nil
}

// visit 对一台做一遍：建连、认证，把结果收成一条事实。
//
// 只写事实，不写结论：连上没连上是事实，「这一台算成功还是失败」由结算站按本模式的成功
// 分类解释（ADR-0035 决定四）。
func visit(ctx context.Context, p *contract.Plan, idx int) contract.NodeResult {
	node := p.Nodes[idx]
	res := contract.NodeResult{IP: node.IP}

	c := &client{node: node, timeout: seconds(p.Run.TimeoutConnect)}
	start := time.Now()
	err := c.connect(ctx)
	res.ConnectCost = time.Since(start)

	if err != nil {
		res.ErrorText = c.errorText(err)
		// 被取消与「目标机自己的毛病」是两件事：前者是外部喊停，记成事实标志。
		res.Cancelled = ctx.Err() != nil
		return res
	}
	res.Connected = true
	// 连上了也把服务端提示交出去（老版无条件写）：密码过期那类提示伴随的是成功的连接，
	// 分类要靠它认出来。屏幕上正常行不显示这一格（呈现那一步的事）。
	res.ErrorText = c.errorText(nil)
	c.close()
	return res
}

// seconds 把纸上按秒写的超时换成 time.Duration。
func seconds(n int) time.Duration {
	return time.Duration(n) * time.Second
}
