package keytool

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

// 覆盖确认那三档走进程出入口：答案从 stdin 来，别处注入不进去（`ask` 的 reader 是整进程
// 一条，住它自己包里）。这里只判使用者看得见的三样——退出码、屏上那一句、rc 文件动没动。
//
// 只在 POSIX 上跑：要一个 rc 文件当现场，Windows 那一侧是注册表（会真改使用者的环境变量）。

// buildBinary 把主包编成一个二进制，放本包的 build/tmp 沙盒（产物不出工作区）。
//
// 按模块路径编、不设 `Dir`：`go build sshfleet2` 从模块内任何目录都认得出主包，于是不必
// 先算出「模块根在哪」——那个算法要么抄 testtmp 那段上溯，要么把仓库布局写进测试，
// 两头都是多一个会漂的说法（2026-11-16，取舍 11-J8）。
func buildBinary(t *testing.T) string {
	t.Helper()
	sandbox := os.Getenv("TMPDIR")
	if sandbox == "" {
		t.Fatalf("TMPDIR 没指到 build/tmp 沙盒：testtmp 找不到 go.mod，或建不出目录")
	}
	bin := filepath.Join(sandbox, "sshfleet2_keytool_test")
	build := exec.Command("go", "build", "-o", bin, "sshfleet2")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("编二进制失败：%v\n%s", err, out)
	}
	return bin
}

// copyDir 把随包那份配置原样抄进现场：塔尖在分叉之前就把整份配置读一遍，缺一格不让跑。
func copyDir(t *testing.T, src, dst string) {
	t.Helper()
	entries, err := os.ReadDir(src)
	if err != nil {
		t.Fatalf("读 %s 失败：%v", src, err)
	}
	if err := os.MkdirAll(dst, 0o755); err != nil {
		t.Fatalf("建 %s 失败：%v", dst, err)
	}
	for _, e := range entries {
		data, err := os.ReadFile(filepath.Join(src, e.Name()))
		if err != nil {
			t.Fatalf("读 %s 失败：%v", e.Name(), err)
		}
		if err := os.WriteFile(filepath.Join(dst, e.Name()), data, 0o644); err != nil {
			t.Fatalf("写 %s 失败：%v", e.Name(), err)
		}
	}
}

// genKeyScene 造一个「本机已有密钥」的现场：一份随包的配置，加临时 HOME 里带主密钥行的 .bashrc。
func genKeyScene(t *testing.T) (dir, home string) {
	t.Helper()
	dir = t.TempDir()
	home = t.TempDir()
	// 随包配置在模块根下：两段相对路径，不出模块，所以与仓库布局无关。
	copyDir(t, filepath.Join("..", "..", "config"), filepath.Join(dir, "config"))
	rc := "# SSHFleet 主密钥\nexport SSHFLEET_KEY='OLD-KEY-FOR-CONFIRM'\n"
	if err := os.WriteFile(filepath.Join(home, ".bashrc"), []byte(rc), 0o644); err != nil {
		t.Fatalf("写 rc 失败：%v", err)
	}
	return dir, home
}

// runGenKey 跑一次 `--gen-key`，答案由 stdin 给（空串 = 读到输入结束）。
func runGenKey(t *testing.T, bin, dir, home, answer string) (string, int) {
	t.Helper()
	cmd := exec.Command(bin, "--gen-key")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "HOME="+home, "SHELL=/bin/bash", "SSHFLEET_KEY=")
	cmd.Stdin = strings.NewReader(answer)
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

var keyLine = regexp.MustCompile(`export SSHFLEET_KEY='([^']*)'`)

// 答「是」：换一把新的，退出 0，屏上说已覆盖。
func TestGenKeyConfirmYesOverwrites(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("rc 文件是 POSIX 形态；Windows 那侧要改注册表，不在自动化里做")
	}
	bin := buildBinary(t)
	dir, home := genKeyScene(t)

	say, code := runGenKey(t, bin, dir, home, "y\n")
	if code != 0 {
		t.Fatalf("答是应以 0 退出，got %d：\n%s", code, say)
	}
	if !strings.Contains(say, "已确认覆盖，正在重新生成主密钥") {
		t.Fatalf("屏上要说已确认覆盖：\n%s", say)
	}
	rc, err := os.ReadFile(filepath.Join(home, ".bashrc"))
	if err != nil {
		t.Fatalf("读 rc 失败：%v", err)
	}
	got := keyLine.FindStringSubmatch(string(rc))
	if got == nil || got[1] == "OLD-KEY-FOR-CONFIRM" {
		t.Fatalf("rc 里的密钥应当换了一把：%q", string(rc))
	}
}

// 答「否」：原密钥保住，退出 0，rc 一个字节不动。
func TestGenKeyConfirmNoKeeps(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("rc 文件是 POSIX 形态；Windows 那侧要改注册表，不在自动化里做")
	}
	bin := buildBinary(t)
	dir, home := genKeyScene(t)
	rcPath := filepath.Join(home, ".bashrc")
	before, err := os.ReadFile(rcPath)
	if err != nil {
		t.Fatalf("读 rc 失败：%v", err)
	}

	say, code := runGenKey(t, bin, dir, home, "n\n")
	if code != 0 {
		t.Fatalf("答否应以 0 退出，got %d：\n%s", code, say)
	}
	if !strings.Contains(say, "已保留原密钥，未做修改") {
		t.Fatalf("屏上要说已保留原密钥：\n%s", say)
	}
	after, err := os.ReadFile(rcPath)
	if err != nil {
		t.Fatalf("读 rc 失败：%v", err)
	}
	if string(before) != string(after) {
		t.Fatalf("答否时 rc 不该动：\nbefore=%q\nafter =%q", before, after)
	}
}

// 读到输入结束：与答否同一收场，退出 0——不是取消、更不是报错。
func TestGenKeyConfirmInputEndKeeps(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("rc 文件是 POSIX 形态；Windows 那侧要改注册表，不在自动化里做")
	}
	bin := buildBinary(t)
	dir, home := genKeyScene(t)

	say, code := runGenKey(t, bin, dir, home, "")
	if code != 0 {
		t.Fatalf("读到输入结束应以 0 退出，got %d：\n%s", code, say)
	}
	if !strings.Contains(say, "已保留原密钥，未做修改") {
		t.Fatalf("读到输入结束要按「否」办：\n%s", say)
	}
	rc, err := os.ReadFile(filepath.Join(home, ".bashrc"))
	if err != nil {
		t.Fatalf("读 rc 失败：%v", err)
	}
	if !strings.Contains(string(rc), "OLD-KEY-FOR-CONFIRM") {
		t.Fatalf("读到输入结束不该换密钥：%q", string(rc))
	}
}
