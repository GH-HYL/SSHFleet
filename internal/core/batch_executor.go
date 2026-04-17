package core

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"sync"
	"time"

	"sshexec_go/internal/log"
	"sshexec_go/internal/ssh"
)

// BatchExecutor 批量任务执行器
type BatchExecutor struct {
	maxConcurrency int
	totalTasks     int
	consumer       *ResultConsumer
	ctx            context.Context
	cancel         func()
}

// NewBatchExecutor 创建执行器
func NewBatchExecutor(maxConcurrency int, totalTasks int, consumer *ResultConsumer, ctx context.Context) *BatchExecutor {
	ctx, cancel := context.WithCancel(ctx)
	if maxConcurrency <= 0 || maxConcurrency > totalTasks {
		maxConcurrency = totalTasks
	}
	return &BatchExecutor{
		maxConcurrency: maxConcurrency,
		totalTasks:     totalTasks,
		consumer:       consumer,
		ctx:            ctx,
		cancel:         cancel,
	}
}

// 单个SSH任务
type SSHTask struct {
	Config  *ssh.SSHConfig
	Command string
}

// 执行所有任务
func (e *BatchExecutor) Run(tasks []*SSHTask, conn net.Conn) {
	log.Zlog.Info(fmt.Sprintf("批量执行器 - 开始执行%d个任务，并发数:%d", e.totalTasks, e.maxConcurrency))
	if e.totalTasks == 0 {
		log.Zlog.Warn("任务数量为0，程序退出，若为符合预期，请排查有效节点数量")
	}

	// 创建任务缓冲通道
	taskChan := make(chan *SSHTask, e.totalTasks)

	// 创建执行结果缓冲通道
	execResultChan := make(chan *ssh.ExecResult, e.totalTasks)

	// 声明go协程等待变量
	var wg sync.WaitGroup

	// 启动go工作协程
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

	// 等待工作协程完成
	go func() {
		wg.Wait()
		close(execResultChan)
	}()

	// 创建执行结果切片（类似Python列表）
	results := make([]*ssh.ExecResult, 0, e.totalTasks)

	// 收集结果
	for res := range execResultChan {
		results = append(results, res)
		// 实时发送进度更新到Python服务端
		e.sendProgressUpdate(conn, len(results), e.totalTasks)
	}

	defer func() {
		// 睡眠0.5秒，确保所有进度更新都发送到Python服务端
		time.Sleep(500 * time.Millisecond)

		// 发送最终完成信号给Python服务端
		completionMsg := map[string]interface{}{
			"type":     "completion",
			"success":  true,
			"total":    e.totalTasks,
			"finished": len(results),
			"results":  results,
		}
		completionJSON, _ := json.Marshal(completionMsg)
		_, err := conn.Write(completionJSON)
		if err != nil {
			log.Zlog.Warn(err.Error())
		}
		// 添加换行符作为消息分隔符
		_, err = conn.Write([]byte("\n"))
		if err != nil {
			log.Zlog.Warn(err.Error())
		}
		log.Zlog.Succ("Monitor Agent - 任务执行完成，发送完成信号到Monitor Server")

		//test("./test.txt", results)

	}()
}

//func test(dir string, results []*ssh.ExecResult) {
//
//	resultsJson, err := json.MarshalIndent(results, "", "    ")
//
//	// 结果文件创建
//	file, err := os.Create(dir)
//	if err != nil {
//		msg := fmt.Sprintf("结果固化 - 创建结果文件失败，错误类型: %T", err)
//		color.Red(msg)
//		log.Zlog.Error(msg)
//	}
//	defer func(file *os.File) {
//		err := file.Close()
//		if err != nil {
//		}
//	}(file)
//
//	// 结果写入
//	_, err = file.WriteString(string(resultsJson))
//	if err != nil {
//		msg := fmt.Sprintf("结果固化 - 结果文件写入失败，错误类型: %T", err)
//		color.Red(msg)
//		log.Zlog.Error(msg)
//	}
//}

// 执行器 - go 协程
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

			log.Zlog.Sugar().Debugf("协程worker - 开始执行，IP:%s，ID：%d", task.Config.IP, id)

			// 创建SSH客户端实例
			client := ssh.NewSSHClient(task.Config)

			// 开始远程连接并执行
			workResult, err := client.ExecuteCommand(task.Command, e.ctx, task.Config.IP)
			if err != nil {
				log.Zlog.Sugar().Errorf("协程worker - 出现异常，IP:%s，ID：%d\n%v", task.Config.IP, id, err)
			}

			// 提交结果到 消费者 通道
			//e.consumer.Submit(workResult)
			e.consumer.consumerRsultChan <- workResult

			// 提交结果到 结果 通道
			e.safeSendResult(execResultChan, workResult)

			log.Zlog.Sugar().Infof("协程worker - 执行结束，IP：%s，ID：%d，执行结果：%t，退出码：%d，结果分类：%s，输出内容：\n%s", task.Config.IP, id, workResult.ExecBool, workResult.ExitCode, workResult.ResultCategory, workResult.Output)

		}
	}
}

// 安全发送结果
func (e *BatchExecutor) safeSendResult(execResultChan chan<- *ssh.ExecResult, workResult *ssh.ExecResult) bool {
	select {
	case execResultChan <- workResult:
		return true
	case <-e.ctx.Done():
		return false
	}
}

// 发送进度更新到Python服务端
func (e *BatchExecutor) sendProgressUpdate(conn net.Conn, completed int, total int) {
	progressMsg := map[string]interface{}{
		"type":      "progress", // 消息类型固定为progress
		"completed": completed,  // 已完成数量
		"total":     total,      // 总数
	}

	progressJSON, err := json.Marshal(progressMsg)
	if err != nil {
		log.Zlog.Error("Monitor Agent - 进度条更新，JSON序列化失败")
		return
	}

	// 添加换行符作为消息分隔符
	progressJSON = append(progressJSON, '\n')

	_, err = conn.Write(progressJSON)
	if err != nil {
		log.Zlog.Error("Monitor Agent - 进度条更新，发送数据到Monitor Server失败")
	}
}
