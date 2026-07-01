package main

import (
	"flag"

	"SSHFleet/internal/httpserver"
)


func main() {
	port := flag.Int("port", 9090, "监听端口")
	logPath := flag.String("log-path", "", "日志文件路径")
	flag.Parse()

	httpserver.Start(*port, *logPath)
}
