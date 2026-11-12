package guard

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// 红线：塔尖的另外两个结局——「起不来」与「拒绝」。
//
// 只判形状，一条值断言都没有：
//   - 起不来：非 0 退出，那一屏带 `[ERROR]` 与 `[function:…]`（`message.Fatal` 的骨架）；
//   - 拒绝：同样非 0 退出，但那不是起不来那一屏；而且**纸不往下递**——本次的归档目录一个
//     都没建（`history/` 下只有工具日志，没有本次的目录）。
//
// 措辞归自定义区与 A/B，这里一个字都不判。这两条补的是 ADR-0034 决定三在**进程层**的落点：
// 「拒绝权只有闸门一处」此前只有类型层与第五类检查兜着，起个进程跑一遍的判据一条都没有。
func TestRedlineTipCannotStart(t *testing.T) {
	run := tipEndingsProbe(t)

	// 空目录：配置读不到，这是「起不来」那条路上最干净的一种。
	say, code := run(t.TempDir(), "-c", "uptime", "-f", "10.0.0.1,22,root,pw")

	if code != 1 {
		t.Errorf("起不来应以 1 退出，got %d：\n%s", code, say)
	}
	if !strings.Contains(say, "[ERROR]") || !strings.Contains(say, "[function:") {
		t.Errorf("起不来那一屏该带 [ERROR] 与 [function:…]：\n%s", say)
	}
}

func TestRedlineTipRefused(t *testing.T) {
	run := tipEndingsProbe(t)

	dir := writeRedlineConfig(t)
	copyRedlineDangerRules(t, dir)

	// `rm -rf /` 是随包危险规则里第一条禁止档：闸门拒绝，塔尖把那一屏打出来就退。
	say, code := run(dir, "-c", "rm -rf /", "-f", "10.0.0.1,22,root,pw")

	if code != 1 {
		t.Errorf("拒绝应以 1 退出，got %d：\n%s", code, say)
	}
	if strings.Contains(say, "[function:") {
		t.Errorf("拒绝不是起不来：那一屏不该带 [function:…]\n%s", say)
	}
	if say == "" {
		t.Error("拒绝时屏幕上一个字都没有")
	}
	entries, err := os.ReadDir(filepath.Join(dir, "history"))
	if err != nil {
		return // 连工具日志目录都没有，也算没建本次归档
	}
	for _, e := range entries {
		if e.IsDir() {
			t.Fatalf("被拒绝的这一趟不该建归档目录（%s）：纸不许往下递", e.Name())
		}
	}
}

// tipEndingsProbe 编一份塔尖的二进制，交回「在某个目录里跑一次」的函数。
//
// 与帮助那条红线各编一份、各写一遍：红线测试不与别处共用假件（ADR-0037），两处各写一遍
// 才能各自独立地红了。
func tipEndingsProbe(t *testing.T) func(dir string, args ...string) (string, int) {
	t.Helper()

	repoRoot, err := filepath.Abs(filepath.Join("..", "..", "..", ".."))
	if err != nil {
		t.Fatalf("定位仓库根失败：%v", err)
	}
	// 产物落 build/ 下沙盒：可执行文件不许出工作区，也不许落在模块根。
	sandbox := filepath.Join(repoRoot, "build", "建站04红线_2026-11-03")
	if err := os.MkdirAll(sandbox, 0o755); err != nil {
		t.Fatalf("建沙盒失败：%v", err)
	}
	// Windows 上可执行文件要带 .exe：`go build -o` 不会自己补，而 exec 找不带扩展名的
	// 路径会直接报 `executable file not found in %PATH%`（找的是 PATHEXT，不是这个文件）。
	bin := filepath.Join(sandbox, "sshfleet2_endings_bin"+exeSuffix())

	build := exec.Command("go", "build", "-o", bin, ".")
	build.Dir = filepath.Join(repoRoot, "modules", "SSHFleet2_Go")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("编二进制失败：%v\n%s", err, out)
	}

	return func(dir string, args ...string) (string, int) {
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
}

// copyRedlineDangerRules 把随包的危险规则表拷进临时配置目录。
//
// 判据表是仓库里的数据、不是被测代码：拿真表跑，`rm -rf /` 才真的落在禁止档上。
func copyRedlineDangerRules(t *testing.T, dir string) {
	t.Helper()

	repoRoot, err := filepath.Abs(filepath.Join("..", "..", "..", ".."))
	if err != nil {
		t.Fatalf("定位仓库根失败：%v", err)
	}
	src := filepath.Join(repoRoot, "modules", "SSHFleet2_Go", "config")
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

// exeSuffix 是这台机器上可执行文件要带的扩展名。
//
// 只为了「编出来能跑」：Windows 的 exec 按 PATHEXT 找文件，不带 `.exe` 的路径它找不到。
// 这跟被测行为无关，所以放在测试辅助里，不进生产代码。
func exeSuffix() string {
	if runtime.GOOS == "windows" {
		return ".exe"
	}
	return ""
}
