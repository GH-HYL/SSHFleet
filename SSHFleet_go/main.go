package main

import (
	"flag"

	"SSHFleet/internal/httpserver"
)

// 编译二进制文件
// $env:GOOs="linux"; go build -o ..\sshexec_py\src\go\batch_ssh; $env:GOOs="windows"; go build -o ..\sshexec_py\src\go\batch_ssh.exe

func main() {
	port := flag.Int("port", 9090, "监听端口")
	logPath := flag.String("log-path", "", "日志文件路径")
	flag.Parse()

	httpserver.Start(*port, *logPath)
}
