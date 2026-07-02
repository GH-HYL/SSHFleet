package core

import (
	"context"
	"fmt"
	"sync"

	"SSHFleet/internal/localfs"
	"SSHFleet/internal/log"
	"SSHFleet/internal/ssh"
)

// BatchUploadExecutor 上传批量任务执行器
type BatchUploadExecutor struct {
	maxConcurrency int
	totalTasks     int
	ctx            context.Context
	cancel         func()
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
func NewBatchUploadExecutor(concurrency int, totalTasks int, ctx context.Context) *BatchUploadExecutor {
	ctx, cancel := context.WithCancel(ctx)
	if concurrency <= 0 || concurrency > totalTasks {
		concurrency = totalTasks
	}
	return &BatchUploadExecutor{
		maxConcurrency: concurrency,
		totalTasks:     totalTasks,
		ctx:            ctx,
		cancel:         cancel,
	}
}

// Run 启动上传执行，返回结果 channel
func (e *BatchUploadExecutor) Run(tasks []*UploadTask) <-chan *ssh.UploadResult {
	log.Zlog.Info(fmt.Sprintf("上传执行器 - 开始执行%d个任务，并发数:%d", e.totalTasks, e.maxConcurrency))
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

			log.Zlog.Sugar().Debugf("上传worker - 开始执行，IP:%s，ID:%d", task.Config.IP, id)

			client := ssh.NewSSHClient(task.Config)
			result, err := client.UploadFiles(task.FileItems, task.RemotePath, task.UseSudo, e.ctx, task.Config.IP)
			if err != nil {
				log.Zlog.Sugar().Errorf("上传worker - 出现异常，IP:%s，ID:%d\n%v", task.Config.IP, id, err)
			}

			result.Seq = task.Seq

			e.safeSendResult(resultChan, result)

			log.Zlog.Sugar().Infof("上传worker - 执行结束，IP:%s，ID:%d，成功:%d/%d", task.Config.IP, id, result.SuccessFiles, result.TotalFiles)
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
