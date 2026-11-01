package guard

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// 红线：塔尖的进程出入口（三种接缝之一，也是唯一允许看内容的地方）。
// 只判形状：`-h` 退出 0、版本行在首行下方；空参数同一屏；配置不在则非 0 退出。
// 措辞对不对不归这里判——那是自定义区与 A/B 比对的事。

func TestRedlineTipRunsHelp(t *testing.T) {
	repoRoot, err := filepath.Abs(filepath.Join("..", "..", "..", ".."))
	if err != nil {
		t.Fatalf("定位仓库根失败：%v", err)
	}
	moduleRoot := filepath.Join(repoRoot, "modules", "SSHFleet2_Go")

	// 产物落 build/ 下当天的沙盒：可执行文件不许出工作区，也不许落在模块根。
	sandbox := filepath.Join(repoRoot, "build", "建站02红线_2026-11-01")
	if err := os.MkdirAll(sandbox, 0o755); err != nil {
		t.Fatalf("建沙盒失败：%v", err)
	}
	bin := filepath.Join(sandbox, "sshfleet2_redline_bin.exe")

	build := exec.Command("go", "build", "-o", bin, ".")
	build.Dir = moduleRoot
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("编二进制失败：%v\n%s", err, out)
	}

	run := func(dir string, args ...string) (string, int) {
		t.Helper()
		cmd := exec.Command(bin, args...)
		cmd.Dir = dir
		var buf strings.Builder
		cmd.Stdout = &buf
		cmd.Stderr = &buf
		err := cmd.Run()
		code := 0
		if exit, ok := err.(*exec.ExitError); ok {
			code = exit.ExitCode()
		} else if err != nil {
			t.Fatalf("起二进制失败：%v", err)
		}
		return buf.String(), code
	}

	// 配置得按本机形态造一份：随包那份是 Linux 形态，在 Windows 上老版也会把它拦下来
	// （既有行为），拿它跑「-h 出帮助」这条红线是测错了对象。假件只住测试，各写一遍。
	runDir := writeRedlineConfig(t)

	say, code := run(runDir, "-h")
	if code != 0 {
		t.Errorf("-h 应以 0 退出，got %d：\n%s", code, say)
	}
	lines := strings.Split(say, "\n")
	if len(lines) < 2 || !strings.HasPrefix(lines[1], "版本: v") {
		t.Errorf("首行下一行应以「版本: v」开头：\n%s", say)
	}

	again, code := run(runDir) // 空参数
	if code != 0 || again != say {
		t.Errorf("空参数应出同一屏并以 0 退出（code %d）", code)
	}

	_, code = run(t.TempDir(), "-h") // 配置不在的那条分叉
	if code == 0 {
		t.Errorf("配置读不到应以非 0 退出")
	}
}

// writeRedlineConfig 造一份读得通的配置并返回那个目录（红线自己的假件，不与自定义区共用）。
func writeRedlineConfig(t *testing.T) string {
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
	if err := os.WriteFile(filepath.Join(dir, "config", "SSHFleet.conf"), []byte(body), 0o644); err != nil {
		t.Fatalf("写临时配置失败：%v", err)
	}
	return dir
}
