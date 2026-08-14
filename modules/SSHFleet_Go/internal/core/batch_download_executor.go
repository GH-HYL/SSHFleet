package core

import (
	"context"

	"SSHFleet/internal/log"
	"SSHFleet/internal/ssh"

	"go.uber.org/zap"
)

// BatchDownloadExecutor 下载批量任务执行器（薄壳：复用泛型批处理骨架 runPool）
type BatchDownloadExecutor struct {
	maxConcurrency int
	totalTasks     int
	ctx            context.Context
	progressChan   chan<- ssh.ProgressMsg
}

// DownloadTask 单个下载任务
type DownloadTask struct {
	Seq        int
	Config     *ssh.SSHConfig
	RemotePath string
	LocalPath  string
	UseSudo    bool
}

// NewBatchDownloadExecutor 创建下载执行器
func NewBatchDownloadExecutor(concurrency int, totalTasks int, ctx context.Context, progressChan chan<- ssh.ProgressMsg) *BatchDownloadExecutor {
	if concurrency <= 0 || concurrency > totalTasks {
		concurrency = totalTasks
	}
	return &BatchDownloadExecutor{
		maxConcurrency: concurrency,
		totalTasks:     totalTasks,
		ctx:            ctx,
		progressChan:   progressChan,
	}
}

// Run 启动下载执行，返回结果 channel
func (e *BatchDownloadExecutor) Run(tasks []*DownloadTask) <-chan *ssh.DownloadResult {
	return runPool(
		e.ctx, e.maxConcurrency, e.totalTasks,
		"下载", "下载任务数量为0",
		tasks, e.progressChan,
		func(id int, task *DownloadTask, onProgress func(ssh.ProgressMsg)) *ssh.DownloadResult {
			log.Zlog.Debug("下载worker - 开始", zap.String("ip", task.Config.IP))

			client := ssh.NewSSHClient(task.Config)
			result, err := client.DownloadFiles(task.RemotePath, task.LocalPath, task.UseSudo, e.ctx, task.Seq, task.Config.IP, onProgress)
			if err != nil && err.Error() != "context canceled" {
				log.Zlog.Error("下载worker - 异常", zap.String("ip", task.Config.IP), zap.Error(err))
			}

			result.Seq = task.Seq

			return result
		},
	)
}
