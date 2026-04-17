package core

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"sshexec_go/internal/log"
	"sshexec_go/internal/ssh"
)

// 结果消费者
type ResultConsumer struct {
	outputDir         string
	file              *os.File
	writer            *bufio.Writer
	mu                sync.Mutex
	consumerRsultChan chan *ssh.ExecResult
	done              chan struct{}
	isRunning         bool
}

// 创建消费者
func NewResultConsumer(outputDir string, num int) *ResultConsumer {
	return &ResultConsumer{
		outputDir:         outputDir,
		consumerRsultChan: make(chan *ssh.ExecResult, num),
		done:              make(chan struct{}),
	}
}

// 启动消费者
func (c *ResultConsumer) Start(outputPath string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.isRunning {
		return nil
	}

	// 创建目录
	if err := os.MkdirAll(c.outputDir, 0755); err != nil {

		return fmt.Errorf("创建输出目录失败: %w", err)
	}

	// 打开文件
	filePath := filepath.Join(c.outputDir, outputPath)
	file, err := os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("打开输出文件失败: %w", err)
	}

	c.file = file
	c.writer = bufio.NewWriter(file)
	c.isRunning = true
	go c.consumeResults()
	log.Zlog.Succ("结果消费者 - 启动成功")
	return nil
}

// 提交结果
// 这个要仔细看一下，怎么会有把结果抛弃的逻辑呢，队列满？resultChan什么时候会满呢,满了就应该阻塞，怎么能丢弃呢
func (c *ResultConsumer) Submit(result *ssh.ExecResult) {
	select {
	case c.consumerRsultChan <- result:
	default:
		log.Zlog.Warn("结果消费者 - 结果队列已满，丢弃结果")
	}
	//c.consumerRsultChan <- result
}

// Stop 停止消费者
func (c *ResultConsumer) Stop() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.isRunning {
		return
	}
	close(c.done)
	c.isRunning = false
}

// 消费结果
func (c *ResultConsumer) consumeResults() {
	for {
		select {
		case <-c.done:
			c.flushAndClose()
			return
		case result := <-c.consumerRsultChan:
			c.processResult(result)
		}
	}
}

// 处理单个结果
func (c *ResultConsumer) processResult(result *ssh.ExecResult) {
	output := c.formatResult(result)
	fmt.Println(output) // 终端输出

	// 文件写入（加锁）
	c.mu.Lock()
	defer c.mu.Unlock()

	if _, err := c.writer.WriteString(output + "\n" + strings.Repeat("=", 50) + "\n"); err != nil {
		log.Zlog.Error(fmt.Sprintf("结果消费者 - 写入结果失败: %v", err))
	}
	_ = c.writer.Flush()
}

// 格式化结果
func (c *ResultConsumer) formatResult(result *ssh.ExecResult) string {
	lines := make([]string, 0, 4)
	connStatus := "成功"
	if !result.ConnectSuccess {
		connStatus = "失败"
	}
	lines = append(lines, fmt.Sprintf("【%s】 连接: %s - %.3fs", result.IP, connStatus, result.ConnCostTime))

	if result.ConnectSuccess {
		execStatus := "成功"
		if !result.ExecBool {
			execStatus = "失败"
		}
		lines = append(lines,
			fmt.Sprintf("【%s】 执行: %s - %.3fs", result.IP, execStatus, result.ExecCostTime),
			result.Output,
		)
	} else {
		lines = append(lines, fmt.Sprintf("【%s】 错误: %s", result.IP, result.ConnectMsg))
	}

	lines = append(lines, fmt.Sprintf("【%s】 分类: %s", result.IP, result.ResultCategory))
	lines = append(lines, strings.Repeat("=", 50))
	return strings.Join(lines, "\n")
}

// 刷新并关闭资源
func (c *ResultConsumer) flushAndClose() {
	if c.writer != nil {
		_ = c.writer.Flush()
	}
	if c.file != nil {
		_ = c.file.Close()
	}
	log.Zlog.Info("结果消费者已停止")
}
