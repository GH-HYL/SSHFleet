package core

import (
	"context"
	"sync"

	"SSHFleet/internal/log"
	"SSHFleet/internal/ssh"

	"go.uber.org/zap"
)

// BatchExecutor 批量任务执行器
type BatchExecutor struct {
	maxConcurrency int
	totalTasks     int
	ctx            context.Context
	cancel         func()
}

// NewBatchExecutor 创建执行器
func NewBatchExecutor(concurrency int, totalTasks int, ctx context.Context) *BatchExecutor {
	ctx, cancel := context.WithCancel(ctx)
	if concurrency <= 0 || concurrency > totalTasks {
		concurrency = totalTasks
	}
	return &BatchExecutor{
		maxConcurrency: concurrency,
		totalTasks:     totalTasks,
		ctx:            ctx,
		cancel:         cancel,
	}
}

// SSHTask 单个SSH任务
type SSHTask struct {
	Seq     int
	Config  *ssh.SSHConfig
	Command string
}

// Run 启动执行，返回结果 channel（执行完毕后自动关闭）
func (e *BatchExecutor) Run(tasks []*SSHTask) <-chan *ssh.ExecResult {
	log.Zlog.Info("批量执行器 - 开始执行", zap.Int("tasks", e.totalTasks), zap.Int("concurrency", e.maxConcurrency))
	if e.totalTasks == 0 {
		log.Zlog.Warn("任务数量为0，程序退出，若为符合预期，请排查有效节点数量")
	}

	taskChan := make(chan *SSHTask, e.totalTasks)
	execResultChan := make(chan *ssh.ExecResult, e.totalTasks)

	var wg sync.WaitGroup

	// 启动工作协程
	for i := 0; i < e.maxConcurrency; i++ {
		wg.Add(1)
		go e.worker(i+1, taskChan, execResultChan, &wg)
	}

	// 提交任务
	go func() {
		defer close(taskChan)
		for _, task := range tasks {
			select {
			case taskChan <- task:
			case <-e.ctx.Done():
				log.Zlog.Warn("批量执行器 - 任务提交被中断")
				return
			}
		}
	}()

	// 等待工作协程完成后关闭结果 channel
	go func() {
		wg.Wait()
		close(execResultChan)
	}()

	return execResultChan
}

// worker 工作协程
func (e *BatchExecutor) worker(id int, taskChan <-chan *SSHTask, execResultChan chan<- *ssh.ExecResult, wg *sync.WaitGroup) {
	defer wg.Done()

	for {
		select {
		case <-e.ctx.Done():
			return
		case task, ok := <-taskChan:
			if !ok {
				return
			}

			log.Zlog.Debug("协程worker - 开始执行", zap.String("ip", task.Config.IP), zap.Int("workerId", id))

			client := ssh.NewSSHClient(task.Config)
			workResult, err := client.ExecuteCommand(task.Command, e.ctx, task.Config.IP)
			if err != nil {
				log.Zlog.Error("协程worker - 出现异常", zap.String("ip", task.Config.IP), zap.Int("workerId", id), zap.Error(err))
			}

			workResult.Seq = task.Seq

			// 不再单独打印"节点输出"，合并到下方的"执行结束"中
			// 异常路径（err != nil）由上方的 Error 日志保留输出

			// 不管成功失败都写入 channel
			e.safeSendResult(execResultChan, workResult)

			// 统一以"执行结束"作为正常路径的收尾日志，与上方"SSH连接成功"风格一致
			if workResult.ExitCode != nil {
				log.Zlog.Info("执行结束", zap.String("ip", task.Config.IP), zap.Int("workerId", id), zap.Int("exitCode", *workResult.ExitCode))
			} else {
				log.Zlog.Info("执行结束", zap.String("ip", task.Config.IP), zap.Int("workerId", id), zap.String("exitCode", "nil"))
			}
		}
	}
}

// safeSendResult 安全发送结果
func (e *BatchExecutor) safeSendResult(execResultChan chan<- *ssh.ExecResult, workResult *ssh.ExecResult) {
	select {
	case execResultChan <- workResult:
	case <-e.ctx.Done():
	}
}
