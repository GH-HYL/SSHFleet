package core

import (
	"context"

	"SSHFleet/internal/localfs"
	"SSHFleet/internal/log"
	"SSHFleet/internal/ssh"

	"go.uber.org/zap"
)

// BatchUploadExecutor 上传批量任务执行器（薄壳：复用泛型批处理骨架 runBatch）
type BatchUploadExecutor struct {
	maxConcurrency int
	totalTasks     int
	ctx            context.Context
	progressChan   chan<- ssh.ProgressMsg
}

// UploadTask 单个上传任务
type UploadTask struct {
	Seq        int
	Config     *ssh.SSHConfig
	FileItems  []localfs.FileItem
	RemotePath string
	UseSudo    bool
}

// NewBatchUploadExecutor 创建上传执行器
func NewBatchUploadExecutor(concurrency int, totalTasks int, ctx context.Context, progressChan chan<- ssh.ProgressMsg) *BatchUploadExecutor {
	if concurrency <= 0 || concurrency > totalTasks {
		concurrency = totalTasks
	}
	return &BatchUploadExecutor{
		maxConcurrency: concurrency,
		totalTasks:     totalTasks,
		ctx:            ctx,
		progressChan:   progressChan,
	}
}

// Run 启动上传执行，返回结果 channel
func (e *BatchUploadExecutor) Run(tasks []*UploadTask) <-chan *ssh.UploadResult {
	return runBatch(
		e.ctx, e.maxConcurrency, e.totalTasks,
		"上传", "上传任务数量为0",
		tasks, e.progressChan,
		func(id int, task *UploadTask, onProgress func(ssh.ProgressMsg)) *ssh.UploadResult {
			log.Zlog.Debug("上传worker - 开始", zap.String("ip", task.Config.IP))

			client := ssh.NewSSHClient(task.Config)
			result, err := client.UploadFiles(task.FileItems, task.RemotePath, task.UseSudo, e.ctx, task.Seq, task.Config.IP, onProgress)
			if err != nil && err.Error() != "context canceled" {
				log.Zlog.Error("上传worker - 异常", zap.String("ip", task.Config.IP), zap.Error(err))
			}

			result.Seq = task.Seq

			return result
		},
	)
}
