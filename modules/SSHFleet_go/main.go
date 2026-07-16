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

	fmt.Printf("环境变量: SSH_FLEET_KEY=%s, SSH_FLEET_PORT=%s, SSH_FLEET_LOG_PATH=%s\n", key, portStr, logPath)

	var missing []string
	if key == "" {
		missing = append(missing, "SSH_FLEET_KEY")
	}
	if portStr == "" {
		missing = append(missing, "SSH_FLEET_PORT")
	}

	if len(missing) > 0 {
		fmt.Printf("错误: 以下环境变量未设置: %v\n", missing)
		os.Exit(1)
	}

	port, err := strconv.Atoi(portStr)
	if err != nil {
		fmt.Printf("错误: SSH_FLEET_PORT 格式无效: %s\n", portStr)
		os.Exit(1)
	}

	fmt.Printf("启动参数: key=***, port=%d, log-path=%q\n", port, logPath)
	httpserver.Start(port, logPath, key)
}
