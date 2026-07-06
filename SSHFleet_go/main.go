package main

import (
	"flag"
	"fmt"

	"SSHFleet/internal/httpserver"
)


func main() {
	port := flag.Int("port", 9090, "监听端口")
	logPath := flag.String("log-path", "", "日志文件路径")
	flag.Parse()

	fmt.Printf("启动参数: port=%d, log-path=%q\n", *port, *logPath)
	httpserver.Start(*port, *logPath)
}
