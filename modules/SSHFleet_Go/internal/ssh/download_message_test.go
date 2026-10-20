//go:build integration

package ssh

// 下载一摊：**远端**打开文件失败挂的是远端环节名，不是本地环节名。
//
// 这条失败点（远端文件存在、`find` 列得出、`sftp.Open` 才被拒）在日常环境里造不出来：
// root 无视权限位，换所有权又要 sudo。所以这里只跑能确定造出来的那一段——远端路径
// 不存在——并断言「本地没参与时报错原文里不得出现本地环节名」。改动前后的差别正在这里：
// 改之前下载侧把 open 的失败挂成 local_file_failed，名字说本地、内容说远端。

import (
	"context"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

// TestDownloadPrecheckFailureIsRemoteSide 预检失败时，报文必须落远端环节名。
func TestDownloadPrecheckFailureIsRemoteSide(t *testing.T) {
	c, ok := integrationClient(t)
	if !ok {
		return
	}

	res := c.DownloadFiles(context.Background(), "/tmp/sshfleet-no-such-path", t.TempDir(), false, 0, nil)
	if res.Error == nil {
		t.Fatalf("远端路径不存在却判成功：%+v", res)
	}
	got := *res.Error
	if strings.Contains(got, "sshfleet.transfer.local_") {
		t.Fatalf("本地没参与，报文却挂了本地环节名：%q", got)
	}
	if !strings.HasPrefix(got, "sshfleet.transfer.remote_missing: ") {
		t.Fatalf("期望 remote_missing 报文，实际 %q", got)
	}
}

// integrationClient 按环境变量建一个客户端；信息不全时 skip。
// 约定见 audit.md §十三：SSHFLEET_TEST_HOST / PORT / USER / PASSWORD。
func integrationClient(t *testing.T) (*Client, bool) {
	t.Helper()
	host := os.Getenv("SSHFLEET_TEST_HOST")
	portStr := os.Getenv("SSHFLEET_TEST_PORT")
	user := os.Getenv("SSHFLEET_TEST_USER")
	pass := os.Getenv("SSHFLEET_TEST_PASSWORD")
	if host == "" || portStr == "" || user == "" || pass == "" {
		t.Skip("集成测试的连接信息不全（SSHFLEET_TEST_HOST/PORT/USER/PASSWORD），跳过")
	}
	port, err := strconv.Atoi(portStr)
	if err != nil || port <= 0 {
		t.Fatalf("SSHFLEET_TEST_PORT 不是合法端口：%q", portStr)
	}
	return NewClient(&Config{IP: host, Port: port, User: user, Password: pass, ConnectTimeout: 10 * time.Second}), true
}
