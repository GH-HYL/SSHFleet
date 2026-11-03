package parse

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// writeTempConfig 造一份能读通的配置并切到那个目录，返回还原用的函数。
//
// 为什么不在模块根上直接读随包那份：随包那份是 Linux 形态（`secret_dir` 是 /home/…），
// 在 Windows 上老版也会把那条路径拦下来——这是既有行为，不是本阶段引入的。要测「读得通」
// 就得按本机形态造一份；这是测试用具，只住测试，不进生产。
func writeTempConfig(t *testing.T) string {
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
	copyKeywords(t, dir)
	t.Chdir(dir)
	return dir
}

// copyKeywords 把随包的三份判据表拷进临时配置目录。
//
// 判据表是**仓库里的数据**，不是被测代码：测试拿真表跑，才验得出「读清单与读表在同一条
// 路上」。源路径必须在 chdir 之前算好——切到临时目录之后，相对路径就指到别处了。
func copyKeywords(t *testing.T, dir string) {
	t.Helper()
	src := keywordsSourceDir(t)
	for _, name := range []string{"keywords_dangerous.conf", "keywords_error.conf", "keywords_passwd.conf"} {
		data, err := os.ReadFile(filepath.Join(src, name))
		if err != nil {
			t.Fatalf("读不到随包判据表 %s：%v", name, err)
		}
		if err := os.WriteFile(filepath.Join(dir, "config", name), data, 0o644); err != nil {
			t.Fatalf("写临时判据表失败：%v", err)
		}
	}
}

// keywordsSourceDir 是随包判据表所在的目录，按**本文件的位置**算绝对路径。
//
// 不按当前工作目录算：一个用例里造两份临时配置很常见（第二份的 cwd 已经是第一份的临时
// 目录），相对路径那时就指到别处了。
func keywordsSourceDir(t *testing.T) string {
	t.Helper()
	_, self, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("拿不到本文件的位置")
	}
	return filepath.Join(filepath.Dir(self), "..", "..", "config")
}

// writeFile 在临时目录里造一个文件，交回它的路径。
func writeFile(t *testing.T, dir, name, body string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("造文件失败：%v", err)
	}
	return path
}
