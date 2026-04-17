package socket

import (
	"net"
	"sshexec_go/internal/log"
	"time"
)

func MonitorAgent(monitorInfo string) (net.Conn, error) {
	var conn net.Conn
	var err error
	maxRetries := 10
	for i := 0; i < maxRetries; i++ {
		conn, err = net.Dial("tcp", monitorInfo)
		if err != nil {
			log.Zlog.Sugar().Infof("Monitor Agent - 第 %d 次重试连接Monitor Server: %s", i+1, monitorInfo)
			time.Sleep(500 * time.Millisecond)
		} else {
			log.Zlog.Succ("Monitor Agent - 连接成功")
			break
		}
	}
	if err != nil {
		return nil, err
	}
	return conn, nil
}
