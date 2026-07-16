package core

import (
	"context"
	"sync"

	"SSHFleet/internal/localfs"
	"SSHFleet/internal/log"
	"SSHFleet/internal/ssh"

	"go.uber.org/zap"
)

// BatchUploadExecutor 上传批量任务执行器
type BatchUploadExecutor struct {
	maxConcurrency int
	totalTasks     int
	ctx            context.Context
	cancel         func()
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
	ctx, cancel := context.WithCancel(ctx)
	if concurrency <= 0 || concurrency > totalTasks {
		concurrency = totalTasks
	}
	return &BatchUploadExecutor{
		maxConcurrency: concurrency,
		totalTasks:     totalTasks,
		ctx:            ctx,
		cancel:         cancel,
		progressChan:   progressChan,
	}
}

// Run 启动上传执行，返回结果 channel
func (e *BatchUploadExecutor) Run(tasks []*UploadTask) <-chan *ssh.UploadResult {
	log.Zlog.Info("上传执行器 - 开始执行", zap.Int("tasks", e.totalTasks), zap.Int("concurrency", e.maxConcurrency))
	if e.totalTasks == 0 {
		log.Zlog.Warn("上传任务数量为0")
	}

	taskChan := make(chan *UploadTask, e.totalTasks)
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
				log.Zlog.Warn("上传执行器 - 任务提交被中断")
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

// worker 上传工作协程
func (e *BatchUploadExecutor) worker(id int, taskChan <-chan *UploadTask, resultChan chan<- *ssh.UploadResult, wg *sync.WaitGroup) {
	defer wg.Done()

	for {
		select {
		case <-e.ctx.Done():
			return
		case task, ok := <-taskChan:
			if !ok {
				return
			}

			log.Zlog.Debug("上传worker - 开始", zap.String("ip", task.Config.IP))

			client := ssh.NewSSHClient(task.Config)
			onProgress := func(msg ssh.ProgressMsg) {
				e.progressChan <- msg
			}
			result, err := client.UploadFiles(task.FileItems, task.RemotePath, task.UseSudo, e.ctx, task.Seq, task.Config.IP, onProgress)
			if err != nil && err.Error() != "context canceled" {
				log.Zlog.Error("上传worker - 异常", zap.String("ip", task.Config.IP), zap.Error(err))
			}

			result.Seq = task.Seq

			e.safeSendResult(resultChan, result)
		}
	}
}

// safeSendResult 安全发送结果
func (e *BatchUploadExecutor) safeSendResult(resultChan chan<- *ssh.UploadResult, result *ssh.UploadResult) {
	select {
	case resultChan <- result:
	case <-e.ctx.Done():
	}
}
