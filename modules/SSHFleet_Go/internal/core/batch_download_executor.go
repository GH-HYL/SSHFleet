package core

import (
	"context"
	"sync"

	"SSHFleet/internal/log"
	"SSHFleet/internal/ssh"

	"go.uber.org/zap"
)

// BatchDownloadExecutor 下载批量任务执行器
type BatchDownloadExecutor struct {
	maxConcurrency int
	totalTasks     int
	ctx            context.Context
	cancel         func()
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
	ctx, cancel := context.WithCancel(ctx)
	if concurrency <= 0 || concurrency > totalTasks {
		concurrency = totalTasks
	}
	return &BatchDownloadExecutor{
		maxConcurrency: concurrency,
		totalTasks:     totalTasks,
		ctx:            ctx,
		cancel:         cancel,
		progressChan:   progressChan,
	}
}

// Run 启动下载执行，返回结果 channel
func (e *BatchDownloadExecutor) Run(tasks []*DownloadTask) <-chan *ssh.UploadResult {
	log.Zlog.Info("下载执行器 - 开始执行", zap.Int("tasks", e.totalTasks), zap.Int("concurrency", e.maxConcurrency))
	if e.totalTasks == 0 {
		log.Zlog.Warn("下载任务数量为0")
	}

	taskChan := make(chan *DownloadTask, e.totalTasks)
	resultChan := make(chan *ssh.UploadResult, e.totalTasks)

	var wg sync.WaitGroup

	for i := 0; i < e.maxConcurrency; i++ {
		wg.Add(1)
		go e.worker(i+1, taskChan, resultChan, &wg)
	}

	go func() {
		defer close(taskChan)
		for _, task := range tasks {
			select {
			case taskChan <- task:
			case <-e.ctx.Done():
				log.Zlog.Warn("下载执行器 - 任务提交被中断")
				return
			}
		}
	}()

	go func() {
		wg.Wait()
		close(resultChan)
	}()

	return resultChan
}

// worker 下载工作协程
func (e *BatchDownloadExecutor) worker(id int, taskChan <-chan *DownloadTask, resultChan chan<- *ssh.UploadResult, wg *sync.WaitGroup) {
	defer wg.Done()

	for {
		select {
		case <-e.ctx.Done():
			return
		case task, ok := <-taskChan:
			if !ok {
				return
			}

			log.Zlog.Debug("下载worker - 开始", zap.String("ip", task.Config.IP))

			client := ssh.NewSSHClient(task.Config)
			onProgress := func(msg ssh.ProgressMsg) {
				e.progressChan <- msg
			}
			result, err := client.DownloadFiles(task.RemotePath, task.LocalPath, task.UseSudo, e.ctx, task.Seq, task.Config.IP, onProgress)
			if err != nil && err.Error() != "context canceled" {
				log.Zlog.Error("下载worker - 异常", zap.String("ip", task.Config.IP), zap.Error(err))
			}

			result.Seq = task.Seq

			e.safeSendResult(resultChan, result)
		}
	}
}

// safeSendResult 安全发送结果
func (e *BatchDownloadExecutor) safeSendResult(resultChan chan<- *ssh.UploadResult, result *ssh.UploadResult) {
	select {
	case resultChan <- result:
	case <-e.ctx.Done():
	}
}
