package credential

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// --convert-secret 的路径来自命令行，基准是当前工作目录（spec D31）：
// 文件就在手边时，转换必须命中它，而不是去 secret_dir 里找同名文件。
func TestConvertSecretResolvesRelativePathAgainstWorkingDir(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	if err := os.WriteFile(filepath.Join(dir, "pass.txt"), []byte("S3cret\n"), 0o644); err != nil {
		t.Fatalf("准备凭据文件失败：%v", err)
	}

	if err := ConvertSecret("pass.txt", false); err != nil {
		t.Fatalf("当前工作目录下的相对路径应直接命中，实际报错：%v", err)
	}
}

// 绝对路径不与任何目录拼接：工作目录里没有这个文件，整条路径原样交给文件系统。
func TestConvertSecretTakesAbsolutePathAsIs(t *testing.T) {
	holding := t.TempDir()
	target := filepath.Join(holding, "pass.txt")
	if err := os.WriteFile(target, []byte("S3cret\n"), 0o644); err != nil {
		t.Fatalf("准备凭据文件失败：%v", err)
	}
	t.Chdir(t.TempDir())

	if err := ConvertSecret(target, false); err != nil {
		t.Fatalf("绝对路径应原样命中，实际报错：%v", err)
	}
}

// 找不到文件时，报错要点出相对路径的基准目录，省掉使用者「到底去哪找的」这一步猜测。
func TestConvertSecretMissingFileNamesWorkingDir(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	err := ConvertSecret("nope.txt", false)
	if err == nil {
		t.Fatal("文件不存在时应报错")
	}
	msg := err.Error()
	if !strings.Contains(msg, "nope.txt") {
		t.Fatalf("报错应含使用者输入的路径，实际：%s", msg)
	}
	if !strings.Contains(msg, dir) {
		t.Fatalf("相对路径的报错应点出基准目录 %s，实际：%s", dir, msg)
	}
}

// ~ 展开是命令行路径的既有规则，改基准时不能顺手丢掉：文件在主目录下，
// 当前工作目录里没有同名文件，只有展开后才找得到。HOME 与 USERPROFILE 都设，
// 两个平台取主目录的来源不同。
func TestConvertSecretExpandsHomeTilde(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	if err := os.WriteFile(filepath.Join(home, "pass.txt"), []byte("S3cret\n"), 0o644); err != nil {
		t.Fatalf("准备凭据文件失败：%v", err)
	}

	if err := ConvertSecret("~/pass.txt", false); err != nil {
		t.Fatalf("~ 应展开为主目录后命中，实际报错：%v", err)
	}
}
