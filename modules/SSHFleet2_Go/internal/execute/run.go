package execute

// 执行站的入口：收下闸门放行的那张纸，按纸上的并发跑完每一台，交回第二张纸。
//
// 每台两步：建连与认证（老版步骤 8 的前半），建起来之后跑那条命令并收回退出码与输出，
// 收齐事实再按判据表定分类（10、11 号单），跑完一台立刻出一行、同时去三个去处（12 号单）。
// 脚本、上传、下载随阶段 05；进度界面随 13。
//
// 出口白名单：这个包对外只露 `Run` 一个函数，纸的类型都在契约包里（ADR-0037 决定三）。
//
// 两个路径参数是本次归档目录里的两个落点（`terminal-output.txt`、`SSHFleetExec.log`），
// 由归档包发、塔尖转手。**收字符串而不是 `archive.Paths`**：站多 import 一个包，红线测试
// 就得跟着 import 第二个包（类六只许钉一个），而本包真正要的也只是两条路径。

import (
	"context"
	"sync"
	"time"

	"sshfleet2/internal/console"
	"sshfleet2/internal/contract"
	"sshfleet2/internal/message"
)

// Run 跑一遍：每台一条事实，按纸上的节点顺序交回。
//
// context 取消时**不再开工**，已开工的那台跑完照常上交（老版 `batch/pool.go` 的口径：
// 取消不等于没执行，目标机可能已经被改过）；压根没开跑的那台不造行，只在总台数里以缺额出现。
//
// 第二个返回值是「起不来」：本阶段只有一处会走到它——两个落点开不出来（目录没建、没权限）。
// 别的前提缺失（配置、清单、凭据）都在上游判过；站与站之间递东西的形状就两种，一张纸或者
// 起不来（ADR-0034 决定三）。
func Run(ctx context.Context, p *contract.Plan, termPath, execLogPath string) (contract.ResultPaper, *contract.CannotStart) {
	paper := contract.ResultPaper{
		Total:     len(p.Nodes),
		Extra:     contract.Extra{Mode: p.Mode},
		Tables:    p.Tables,
		RunMeta:   p.RunMeta,
		Resources: p.Resources,
		StartedAt: time.Now(),
	}

	// 屏幕那一路：流从 console 拿（执行站是唯一可以拿它的包），进度界面懒起来。
	scr := newScreen(console.Writer())
	defer scr.stop()

	rep, err := newReporter(scr, termPath, execLogPath, p.Mode)
	if err != nil {
		return contract.ResultPaper{}, &contract.CannotStart{Where: contract.WhereOutput, Body: err.Error()}
	}
	defer rep.close()

	// 传输类才有的那块逐节点面板：聚合器把各节点的字节与文件数汇成快照，
	// 渲染交给屏幕那一路（非终端时面板不出，明细直接落流）。
	var ag *aggregator
	if isTransfer(p.Mode) {
		ag = newAggregator(func(s snapshot) { scr.transfer(s, p.Mode, paper.Total, paper.StartedAt) })
	}

	// 实际生效的并发数：0 表示不填，按全部并行（老版同口径）。
	limit := p.Run.Concurrency
	if limit <= 0 || limit > len(p.Nodes) {
		limit = len(p.Nodes)
	}
	// 生效值落到纸上：报告印的是它（闸门那一问可能把 Run.Concurrency 改过，
	// 而运行上下文那段密文封在改之前）。
	paper.Concurrency = limit

	// 开工前先把「这一轮怎么跑、发下去的是什么」记进执行期日志：命令行里只剩固定形态的
	// `sh -c`，事后没有这一段就查不出原始命令与执行身份（老版写在协程起来之前，同一处）。
	logSessionHead(rep.lg, p, limit)

	if len(p.Nodes) == 0 {
		paper.Interrupted = ctx.Err() != nil
		paper.StoppedAt = paper.StartedAt
		return paper, nil
	}

	// 一开工就把面板摆出来（0 台完成）：老版也是执行一开始就有那块界面，而不是等第一台跑完。
	scr.progress(0, 0, 0, paper.Total, paper.StartedAt)

	// 每台一格。worker 各写自己那一格，互不相干；保序靠下标，不靠收上来的先后。
	type slot struct {
		res    contract.NodeResult
		filled bool
	}
	slots := make([]slot, len(p.Nodes))

	// 进度面板的数字：跑完几台、成了几台、败了几台。只喂面板，不进纸。
	var (
		statMu    sync.Mutex
		completed int
		succeeded int
		failed    int
	)

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
					res := visit(ctx, p, idx, ag)
					slots[idx] = slot{res: res, filled: true}

					// 先推进度、再出那一行：界面起来了，那一行才会经由它打到面板上方
					// （老版也是这个次序——面板在底部，明细往上滚）。
					statMu.Lock()
					completed++
					if message.IsSuccess(res, p.Mode) {
						succeeded++
					} else {
						failed++
					}
					c, s, f := completed, succeeded, failed
					statMu.Unlock()
					scr.progress(c, s, f, paper.Total, paper.StartedAt)
					rep.result(res, p.Nodes[idx].User)
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
	// 这次是不是被按停：事实由本站在收尾时写一格，塔尖读纸（ADR-0042 决定二）。
	// 两处各算一次就是同一个事实两个产出地——塔尖不再自己看 ctx。
	paper.Interrupted = ctx.Err() != nil
	paper.StoppedAt = time.Now()
	return paper, nil
}

// visit 对一台做一遍：建连、认证、跑那条命令，收成一条事实，定一个分类。
//
// 事实与分类都在这一处收口：连不上的那台也要定分类——它的报错原文就是分类依据，而「这一台
// 算成功还是失败」仍不在这里（结算站拿分类去对，ADR-0035 决定四）。
//
// 出那一行在 `Run` 里紧跟着做完（先推进度、再出行），不攒到最后一起打——中途被强杀时
// 盘上已经有痕迹。
func visit(ctx context.Context, p *contract.Plan, idx int, ag *aggregator) contract.NodeResult {
	node := p.Nodes[idx]
	res := contract.NodeResult{IP: node.IP, Port: node.Port, User: node.User}

	c := &client{node: node, timeout: seconds(p.Run.TimeoutConnect)}
	start := time.Now()
	err := c.connect(ctx)
	res.ConnectCost = time.Since(start)

	if err != nil {
		res.ErrorText = c.errorText(err)
		// 被取消与「目标机自己的毛病」是两件事：前者是外部喊停，记成事实标志。
		// 这时它还没被连上过，报文说「连接还没建成就被中断」——与跑到一半那一档分开。
		res.Cancelled = ctx.Err() != nil
		if res.Cancelled {
			res.ErrorText = appendText(res.ErrorText, abortMessage(false))
		}
	} else {
		res.Connected = true
		// 连上了也把服务端提示交出去（老版无条件写）：密码过期那类提示伴随的是成功的连接，
		// 分类要靠它认出来。屏幕上正常行不显示这一格（呈现那一步的事）。
		res.ErrorText = c.errorText(nil)
		res.AuthMethod = c.authMethodDesc()

		// 连上之后按模式分派（建站 05 的第五节）：每一支一个文件，不在这里堆 if。
		// 建连、收尾、分类仍由本函数收口——那是 04 定的，不动。
		if err := c.step(ctx, p, &res, ag); err != nil {
			res.ErrorText = c.mergeBanner(err.Error())
		}

		c.close()
	}

	// 事实收齐了才定分类：两条路都要定，不然连不上的那些台在结算站那里没有名字。
	// 服务端提示另递一份：改密的分类不看它（见 categoryOf）。
	res.Category = categoryOf(p.Mode, p.Tables, &res, c.banner)
	return res
}

// seconds 把纸上按秒写的超时换成 time.Duration。
func seconds(n int) time.Duration {
	return time.Duration(n) * time.Second
}
