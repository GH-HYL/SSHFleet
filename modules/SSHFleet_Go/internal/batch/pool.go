package batch

import (
	"context"
	"sync"

	"sshfleet/internal/ssh"
)

// runPool worker pool：并发执行任务，结果收集后返回（按完成顺序，调用方自行保序）。
// ctx 取消时不再启动新任务、结果静默丢弃（对位旧 runPool 语义）。
func runPool(ctx context.Context, concurrency int, tasks []*task, work func(context.Context, *task) ssh.Result, onResult func(ssh.Result)) []ssh.Result {
	taskCh := make(chan *task, len(tasks))
	resCh := make(chan ssh.Result, len(tasks))

	var wg sync.WaitGroup
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case t, ok := <-taskCh:
					if !ok {
						return
					}
					res := work(ctx, t)
					// 对位旧 runPool：取消后完成的结果不再上报（已完成的节点结果此前已入库）
					select {
					case resCh <- res:
					case <-ctx.Done():
					}
				}
			}
		}()
	}

	go func() {
		defer close(taskCh)
		for _, t := range tasks {
			select {
			case taskCh <- t:
			case <-ctx.Done():
				return
			}
		}
	}()

	go func() {
		wg.Wait()
		close(resCh)
	}()

	// 收集循环是单线程的：结果流水（output.txt / 执行日志 / 终端明细）在此回放，
	// 取消时被丢弃的结果自然不会被写出（对位旧实现只处理收到的结果）。
	results := make([]ssh.Result, 0, len(tasks))
	for r := range resCh {
		results = append(results, r)
		if onResult != nil {
			onResult(r)
		}
	}
	return results
}
