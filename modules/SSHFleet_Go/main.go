package main

import (
	"fmt"
	"os"
	"strconv"

	"SSHFleet/internal/httpserver"
)

func main() {
	key := os.Getenv("SSH_FLEET_KEY")
	portStr := os.Getenv("SSH_FLEET_PORT")
	logPath := os.Getenv("SSH_FLEET_LOG_PATH")

	// 主密钥只在环境变量与 SSH_FLEET_LOG_PATH 中传递，不打印到 stdout
	var missing []string
	if key == "" {
		missing = append(missing, "SSH_FLEET_KEY")
	}
	if portStr == "" {
		missing = append(missing, "SSH_FLEET_PORT")
	}

	if len(missing) > 0 {
		fmt.Fprintf(os.Stderr, "错误: 以下环境变量未设置: %v\n", missing)
		os.Exit(1)
	}

	port, err := strconv.Atoi(portStr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "错误: SSH_FLEET_PORT 格式无效: %s\n", portStr)
		os.Exit(1)
	}

	// 启动细节（端口/日志路径）由日志模块在服务初始化时记录（见 httpserver.Start）
	httpserver.Start(port, logPath, key)
}
