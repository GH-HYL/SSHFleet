package parse

import (
	"os"
	"path/filepath"
	"testing"
)

// writeTempConfig 造一份能读通的配置并切到那个目录，返回还原用的函数。
//
// 为什么不在模块根上直接读随包那份：随包那份是 Linux 形态（`secret_dir` 是 /home/…），
// 在 Windows 上老版也会把那条路径拦下来——这是既有行为，不是本阶段引入的。要测「读得通」
// 就得按本机形态造一份；这是测试用具，只住测试，不进生产。
func writeTempConfig(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "config"), 0o755); err != nil {
		t.Fatalf("建临时配置目录失败：%v", err)
	}
	body := `[account]
port = 22
user = "root"
password = "pw"

[credential]
encrypt = false
key = ""
key_password = ""
secret_dir = ""

[execution]
sudo = true
timeout_connect = 10
timeout_execute = 60
timeout_transfer = 300

[enable]
output_to_xlsx = false
results_to_xlsx = false
show_category_tips = true

[interactive]
regex = false
case_sensitive = false

[env]
PATH = "/usr/bin"

[locale]
english = false

[interpreter]
command = "bash"

[interpreter.script]
".sh" = "bash"

[upload]
small_file = 1048576
large_file = 10485760
medium_parallel = 4
`
	if err := os.WriteFile(filepath.Join(dir, configPath), []byte(body), 0o644); err != nil {
		t.Fatalf("写临时配置失败：%v", err)
	}
	t.Chdir(dir)
}
