//go:build integration

package execute

// 集成用例的底座：造一张**真纸**——argv 交给解析站，回来的就是执行站要吃的那张。
//
// 为什么不手工填 `Downlink`／`Stdin`：下发行的拼装在 02 号单搬回解析站之后，手工填的那一份
// 与真发出去的那一份是两个来源；两边一旦漂移，用例照旧绿（建站 05 开工前的命令用例正是
// 这么红的——`Downlink` 空着，远端跑了个空命令，还判「执行成功」）。
//
// 配置与清单都在临时目录里造，`t.Chdir` 过去让解析站按 `./config/SSHFleet.conf` 找到它。
// 判据表用随包那三份真表，不在这里另写一份。

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"sshfleet2/internal/contract"
	"sshfleet2/internal/parse"
)

// testConfig 是本机形态的配置：加密关、以登录用户执行、提示语英文。
//
// @USER@ 与 @PASS@ 由环境变量填：清单里第 3、4 列留空时靠它兜底，而集成用例不该把口令
// 写进仓库。
const testConfig = `[account]
port     = 22
user     = "@USER@"
password = "@PASS@"

[credential]
encrypt      = false
key          = ""
key_password = ""
secret_dir   = ""

[execution]
sudo             = false
timeout_connect  = 10
timeout_execute  = 60
timeout_transfer = 300

[enable]
output_to_xlsx     = false
results_to_xlsx    = false
show_category_tips = true

[interactive]
regex          = false
case_sensitive = false

[env]
PATH = "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"
TERM = "dumb"

[locale]
english = true

[interpreter]
command = "bash"

[interpreter.script]
".sh" = "bash"
".py" = "python3"

[upload]
small_file      = 2097152
large_file      = 20971520
medium_parallel = 10
`

// planFor 按真 argv 造一张纸（配置用出厂那份的形态）。
func planFor(t *testing.T, nodes []contract.Node, args ...string) *contract.Plan {
	t.Helper()
	return planForTweaked(t, nodes, nil, args...)
}

// planForTweaked 同上，另给一个改配置正文的机会（要在 `[env]` 里多导一条这类）。
func planForTweaked(t *testing.T, nodes []contract.Node, tweak func(string) string, args ...string) *contract.Plan {
	t.Helper()
	_, _, user, pass := integrationCore(t)

	dir := t.TempDir()
	body := strings.ReplaceAll(testConfig, "@USER@", user)
	body = strings.ReplaceAll(body, "@PASS@", pass)
	if tweak != nil {
		body = tweak(body)
	}
	writeTestConfig(t, dir, body)

	manifest := filepath.Join(dir, "nodes.csv")
	var b strings.Builder
	for _, n := range nodes {
		fmt.Fprintf(&b, "%s,%s,%s,%s\n", n.IP, n.Port, n.User, n.Password)
	}
	if err := os.WriteFile(manifest, []byte(b.String()), 0o644); err != nil {
		t.Fatalf("写临时清单失败：%v", err)
	}

	t.Chdir(dir)
	_, plan, fail := parse.Run(append([]string{"sshfleet2", "-f", manifest}, args...), "integration")
	if fail != nil {
		t.Fatalf("解析站起不来：%s", fail.Body)
	}
	if plan == nil {
		t.Fatal("解析站既没给纸也没说起不来")
	}
	return plan
}

// writeTestConfig 把配置与三份判据表铺进临时目录。
func writeTestConfig(t *testing.T, dir, body string) {
	t.Helper()
	cfgDir := filepath.Join(dir, "config")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatalf("建临时配置目录失败：%v", err)
	}
	if err := os.WriteFile(filepath.Join(cfgDir, "SSHFleet.conf"), []byte(body), 0o644); err != nil {
		t.Fatalf("写临时配置失败：%v", err)
	}
	src := keywordsSourceDir(t)
	for _, name := range []string{"keywords_dangerous.conf", "keywords_error.conf", "keywords_passwd.conf"} {
		data, err := os.ReadFile(filepath.Join(src, name))
		if err != nil {
			t.Fatalf("读不到随包判据表 %s：%v", name, err)
		}
		if err := os.WriteFile(filepath.Join(cfgDir, name), data, 0o644); err != nil {
			t.Fatalf("写临时判据表失败：%v", err)
		}
	}
}

// keywordsSourceDir 按本文件位置算随包 config 目录。
//
// 不按当前工作目录算：一个用例里造两份临时配置很常见，第二份时 cwd 已经是第一份的目录。
func keywordsSourceDir(t *testing.T) string {
	t.Helper()
	_, self, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("拿不到本文件的位置")
	}
	return filepath.Join(filepath.Dir(self), "..", "..", "config")
}

// withEnvEntry 在 `[env]` 段里加一行（tweak 用）。
func withEnvEntry(line string) func(string) string {
	return func(body string) string {
		return strings.Replace(body, "[env]\n", "[env]\n"+line+"\n", 1)
	}
}

// enableSudo 把配置里的执行身份改成 root（`[execution] sudo = true`）。
func enableSudo(body string) string {
	return strings.Replace(body, "sudo             = false", "sudo             = true", 1)
}

// setExecuteTimeout 改配置里的执行超时（秒）。
func setExecuteTimeout(sec int) func(string) string {
	return func(body string) string {
		return strings.Replace(body, "timeout_execute  = 60", fmt.Sprintf("timeout_execute  = %d", sec), 1)
	}
}

// manyOneNode 复制同一台若干个（容器只有一个，要验多台并行就是它）。
func manyOneNode(node contract.Node, n int) []contract.Node {
	out := make([]contract.Node, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, node)
	}
	return out
}

// remoteRun 在目标机上跑一条命令，交回那一台的事实。
//
// 传输类用例要用它摆现场（建目录、读回来核对、清场）：走的是同一条下发路径，
// 不另起一个 ssh 客户端。
func remoteRun(t *testing.T, node contract.Node, command string) contract.NodeResult {
	t.Helper()
	return runOne(t, context.Background(), planFor(t, []contract.Node{node}, "-c", command))
}

// remoteTempDir 造一个本次专用的远端目录（在给定的可写根下），并登记收尾清理。
func remoteTempDir(t *testing.T, node contract.Node, base string) string {
	t.Helper()
	dir := fmt.Sprintf("%s/it-%d", base, time.Now().UnixNano())
	if item := remoteRun(t, node, "mkdir -p "+dir); !item.Connected || item.ExitCode == nil || *item.ExitCode != 0 {
		t.Fatalf("建远端目录 %s 失败：%+v", dir, item)
	}
	t.Cleanup(func() {
		// 带 sudo：`--sudo` 上传落下去的文件属 root，普通账号删不掉。
		remoteRun(t, node, "sudo rm -rf "+dir)
	})
	return dir
}
