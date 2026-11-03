//go:build integration

package execute

// 对着真机器跑：连得上、密码错、端口不通、以及「认证阶段服务端有话要说」那一台。
//
// 变量不全就整批 skip（约定同老版：SSHFLEET_TEST_HOST / PORT / USER / PASSWORD）。
// 过期账号那一条另要 SSHFLEET_TEST_EXPIRED_USER / SSHFLEET_TEST_EXPIRED_PASSWORD，
// 造法见 `docs/测试环境使用指南.md` 第四节。

import (
	"context"
	"os"
	"strings"
	"testing"

	"sshfleet2/internal/contract"
)

func TestConnectSucceeds(t *testing.T) {
	node, _ := integrationTarget(t)

	paper, fail := runPlan(t, context.Background(), planFor(t, []contract.Node{node}, "-c", "uptime"))
	if fail != nil {
		t.Fatalf("不该起不来：%+v", fail)
	}
	item := onlyItem(t, paper)
	if !item.Connected {
		t.Fatalf("应能连上，错误原文 %q", item.ErrorText)
	}
	if item.ErrorText != "" {
		t.Fatalf("干净连上时不该有错误原文，实际 %q", item.ErrorText)
	}
}

func TestConnectWrongPasswordIsAuthFailed(t *testing.T) {
	node, _ := integrationTarget(t)
	node.Password = "definitely-not-the-password"

	paper, _ := runPlan(t, context.Background(), planFor(t, []contract.Node{node}, "-c", "uptime"))
	item := onlyItem(t, paper)
	if item.Connected {
		t.Fatal("密码错却记成连上了")
	}
	if !strings.HasPrefix(item.ErrorText, "sshfleet.connect.auth_failed: ") {
		t.Fatalf("应报「身份验证失败」报文，实际 %q", item.ErrorText)
	}
}

func TestConnectClosedPortIsRefused(t *testing.T) {
	node, _ := integrationTarget(t)
	node.Port = deadPort(t)

	paper, _ := runPlan(t, context.Background(), planFor(t, []contract.Node{node}, "-c", "uptime"))
	item := onlyItem(t, paper)
	if item.Connected {
		t.Fatal("端口没人听却记成连上了")
	}
	if !strings.HasPrefix(item.ErrorText, "sshfleet.connect.refused: ") {
		t.Fatalf("应报「拒绝网络连接」报文，实际 %q", item.ErrorText)
	}
}

// TestConnectCarriesServerBanner 认证阶段服务端下的那句提示（PAM 原话）要进结果。
//
// 两种过期造法都会有提示，但结果不同：账号到期（`chage -E`）是连不上、提示当原因；
// 密码到期（`chage -d 0`）是**连得上**、提示要拿去分类。所以这里不断言连接成败，
// 只断言提示进了「错误原文」。
//
// 措辞随目标机的语言与发行版变：Ubuntu 的 pam 不带中文译文，是英文；中文环境下也可能
// 是「您的帐户已过期」。三种写法任一命中即可。
func TestConnectCarriesServerBanner(t *testing.T) {
	integrationCore(t) // 核心四样缺一个就整批 skip

	node := contract.Node{
		IP:       integrationEnv(t, "SSHFLEET_TEST_HOST"),
		Port:     integrationEnv(t, "SSHFLEET_TEST_PORT"),
		User:     integrationEnv(t, "SSHFLEET_TEST_EXPIRED_USER"),
		Password: integrationEnv(t, "SSHFLEET_TEST_EXPIRED_PASSWORD"),
	}

	paper, _ := runPlan(t, context.Background(), planFor(t, []contract.Node{node}, "-c", "uptime"))
	item := onlyItem(t, paper)

	hints := []string{
		"Your account has expired",
		"您的帐户已过期",
		"You are required to change your password immediately",
	}
	for _, hint := range hints {
		if strings.Contains(item.ErrorText, hint) {
			t.Logf("这一台的结果：连上=%v，原文 %q", item.Connected, item.ErrorText)
			return
		}
	}
	t.Fatalf("过期账号的服务端提示没进结果：%q", item.ErrorText)
}

// integrationTarget 按环境变量造一台机器，连同它的密码一起交回。
func integrationTarget(t *testing.T) (contract.Node, string) {
	t.Helper()
	host, port, user, pass := integrationCore(t)
	return contract.Node{IP: host, Port: port, User: user, Password: pass}, pass
}

// integrationCore 取核心四样；缺一样就整批 skip（不是缺哪样跳哪样）。
func integrationCore(t *testing.T) (host, port, user, pass string) {
	t.Helper()
	host = os.Getenv("SSHFLEET_TEST_HOST")
	port = os.Getenv("SSHFLEET_TEST_PORT")
	user = os.Getenv("SSHFLEET_TEST_USER")
	pass = os.Getenv("SSHFLEET_TEST_PASSWORD")
	if host == "" || port == "" || user == "" || pass == "" {
		t.Skip("集成测试的连接信息不全（SSHFLEET_TEST_HOST/PORT/USER/PASSWORD），跳过")
	}
	return host, port, user, pass
}

func integrationEnv(t *testing.T, name string) string {
	t.Helper()
	v := os.Getenv(name)
	if v == "" {
		t.Skipf("集成测试的 %s 没有设置，跳过", name)
	}
	return v
}

func onlyItem(t *testing.T, paper contract.ResultPaper) contract.NodeResult {
	t.Helper()
	if len(paper.Items) != 1 {
		t.Fatalf("应有一条事实，实际 %d 条", len(paper.Items))
	}
	return paper.Items[0]
}
