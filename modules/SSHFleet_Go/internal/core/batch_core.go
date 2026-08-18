package core

import (
	"context"
	"sync"

	"SSHFleet/internal/log"
	"SSHFleet/internal/ssh"

	"go.uber.org/zap"
)

// runPool 泛型批处理骨架：三个 executor 共享的 worker-pool 逻辑只写一份。
// 差异点（任务类型 TASK、结果类型 RESULT、具体工作 work）由调用方闭包注入。
//
// 参数：
//   - ctx:            取消信号（外部传入，worker/提交/发送均监听）
//   - maxConcurrency: 最大并发 worker 数
//   - totalTasks:     任务总数（决定 channel 缓冲与并发上限）
//   - name:           日志前缀（"批量"/"上传"/"下载"，拼出"XX执行器 - ..."）
//   - zeroTasksLog:   任务数为 0 时的日志文案（execute 与其他两个不同，保留原文案）
//   - tasks:          任务列表
//   - progressChan:   进度通道（execute 传 nil；upload/download 传真实通道）
//   - work:           单个任务的实际执行逻辑（worker 协程内调用）
func runPool[TASK any, RESULT any](
	ctx context.Context,
	maxConcurrency int,
	totalTasks int,
	name string,
	zeroTasksLog string,
	tasks []*TASK,
	progressChan chan<- ssh.ProgressMsg,
	work func(id int, task *TASK, onProgress func(ssh.ProgressMsg)) *RESULT,
) <-chan *RESULT {
	log.Zlog.Info(name+"执行器 - 开始执行", zap.Int("tasks", totalTasks), zap.Int("concurrency", maxConcurrency))
	if totalTasks == 0 {
		log.Zlog.Warn(zeroTasksLog)
	}

	taskChan := make(chan *TASK, totalTasks)
	resultChan := make(chan *RESULT, totalTasks)

	var wg sync.WaitGroup

	// 启动工作协程
	for i := 0; i < maxConcurrency; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case task, ok := <-taskChan:
					if !ok {
						return
					}

					// 进度回调：仅 upload/download 有真实通道，execute 传 nil。
					// 取消防护：通道缓冲满且上下文取消时不再阻塞（select 不豁免
					// send-on-closed-channel panic——关闭通道前必须保证无发送者，
					// 由调用方按「取消 → 排空结果流 → 关闭通道」的顺序保证）
					var onProgress func(ssh.ProgressMsg)
					if progressChan != nil {
						onProgress = func(msg ssh.ProgressMsg) {
							select {
							case progressChan <- msg:
							case <-ctx.Done():
							}
						}
					}

					result := work(id, task, onProgress)

					// 不管成功失败都写入 channel（取消时静默丢弃）
					select {
					case resultChan <- result:
					case <-ctx.Done():
					}
				}
			}
		}(i + 1)
	}

	// 提交任务
	go func() {
		defer close(taskChan)
		for _, task := range tasks {
			select {
			case taskChan <- task:
			case <-ctx.Done():
				log.Zlog.Warn(name + "执行器 - 任务提交被中断")
				return
			}
		}
	}()

	// 等待工作协程完成后关闭结果 channel
	go func() {
		wg.Wait()
		close(resultChan)
	}()

	return resultChan
}
