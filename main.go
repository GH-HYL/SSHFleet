package main

import (
	"fmt"
	"io"
	"net"
	"os"
	"time"

	"sshexec_go/internal/core"
	"sshexec_go/internal/interrupt"
	"sshexec_go/internal/jsonproc"
	"sshexec_go/internal/log"
	"sshexec_go/internal/socket"
	"sshexec_go/internal/ssh"

	"github.com/fatih/color"
)

// 编译二进制文件
// $env:GOOs="linux"; go build -o ..\sshexec_py\src\go\batch_ssh; $env:GOOs="windows"; go build -o ..\sshexec_py\src\go\batch_ssh.exe

func main() {

	input, err := io.ReadAll(os.Stdin)
	if err != nil {
		msg := fmt.Sprintf("%s - %s；错误类型: %T", "请求数据读取", "读取标准输入失败", err)
		color.Red(msg)
		panic(err)
	}

	// 将字节切片转换为字符串
	requestJSON := string(input)

	// 解析JSON请求数据
	requestData := jsonproc.ParseRequestJSON(requestJSON)

	// 初始化日志系统
	if err := log.InitLogger(requestData.ExecLogDir, requestData.Config.Paths.Logs.Exec); err != nil {
		msg := fmt.Sprintf("%s - %s；错误类型: %T", "工具初始化", "日志初始化失败", err)
		color.Red(msg)
		panic(err)
	}
	log.Zlog.Succ("日志记录器 - 初始化成功")
	log.Zlog.Sugar().Infof("本次执行的命令：%s", requestData.FinalCommand)

	// 连接Monitor Server
	var conn net.Conn
	if conn, err = socket.MonitorAgent(requestData.MonitorInfo); err != nil {
		msg := fmt.Sprintf("%s - %s；错误类型: %T", "Monitor Agent", "连接失败", err)
		color.Red(msg)
		log.Zlog.Error(msg)
		panic(err)
	}
	defer func(conn net.Conn) {
		err := conn.Close()
		if err != nil {

		}
	}(conn)

	// 创建中断器
	interruptHandler := interrupt.NewInterruptHandler()
	interruptHandler.Setup()

	// 构建任务列表
	var tasks []*core.SSHTask
	for _, node := range requestData.NodesInfo {
		config := &ssh.SSHConfig{
			IP:             node.IP,
			Port:           node.Port,
			User:           node.User,
			Password:       node.Password,
			PrivateKeyPath: requestData.CommandArgs.PrivateKey,
			ConnectTimeout: time.Duration(requestData.CommandArgs.TimeoutConnect) * time.Second,
			ExecTimeout:    time.Duration(requestData.CommandArgs.TimeoutExecute) * time.Second,
		}
		task := &core.SSHTask{
			Config:  config,
			Command: requestData.FinalCommand,
		}
		tasks = append(tasks, task)
	}

	// 创建结果消费者
	consumer := core.NewResultConsumer(requestData.ExecLogDir, len(tasks))

	// 启动结果消费者
	if err := consumer.Start(requestData.Config.Paths.Files.Output); err != nil {
		msg := fmt.Sprintf("结果消费者 - 启动失败，错误类型: %T", err)
		color.Red(msg)
		log.Zlog.Error(msg)
		panic(msg)
	}

	// 创建批量执行器
	totalTasks := len(tasks)
	maxConcurrency := requestData.CommandArgs.Number
	batchExecutor := core.NewBatchExecutor(maxConcurrency, totalTasks, consumer, interruptHandler.Context())

	// 执行任务
	batchExecutor.Run(tasks, conn)

	// 等待0.5秒确保数据发送完成
	// time.Sleep(1 * time.Second)
	time.Sleep(500 * time.Millisecond)
}
