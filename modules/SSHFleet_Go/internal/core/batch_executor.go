package core

import (
	"context"

	"SSHFleet/internal/log"
	"SSHFleet/internal/ssh"

	"go.uber.org/zap"
)

// BatchExecutor 批量命令执行器（薄壳：复用泛型批处理骨架 runPool）
type BatchExecutor struct {
	maxConcurrency int
	totalTasks     int
	ctx            context.Context
}

// SSHTask 单个SSH任务
type SSHTask struct {
	Seq     int
	Config  *ssh.SSHConfig
	Command string
}

// NewBatchExecutor 创建执行器
func NewBatchExecutor(concurrency int, totalTasks int, ctx context.Context) *BatchExecutor {
	if concurrency <= 0 || concurrency > totalTasks {
		concurrency = totalTasks
	}
	return &BatchExecutor{
		maxConcurrency: concurrency,
		totalTasks:     totalTasks,
		ctx:            ctx,
	}
}

// Run 启动执行，返回结果 channel（执行完毕后自动关闭）
func (e *BatchExecutor) Run(tasks []*SSHTask) <-chan *ssh.ExecResult {
	return runPool(
		e.ctx, e.maxConcurrency, e.totalTasks,
		"批量", "任务数量为0，程序退出，若为符合预期，请排查有效节点数量",
		tasks, nil,
		func(id int, task *SSHTask, _ func(ssh.ProgressMsg)) *ssh.ExecResult {
			log.Zlog.Debug("协程worker - 开始执行", zap.String("ip", task.Config.IP), zap.Int("workerId", id))

			client := ssh.NewSSHClient(task.Config)
			workResult, err := client.ExecuteCommand(task.Command, e.ctx, task.Config.IP)
			if err != nil {
				log.Zlog.Error("协程worker - 出现异常", zap.String("ip", task.Config.IP), zap.Int("workerId", id), zap.Error(err))
			}

			workResult.Seq = task.Seq

			// 统一以"执行结束"作为正常路径的收尾日志
			if workResult.ExitCode != nil {
				log.Zlog.Info("执行结束", zap.String("ip", task.Config.IP), zap.Int("workerId", id), zap.Int("exitCode", *workResult.ExitCode))
			} else {
				log.Zlog.Info("执行结束", zap.String("ip", task.Config.IP), zap.Int("workerId", id), zap.String("exitCode", "nil"))
			}

			return workResult
		},
	)
}
