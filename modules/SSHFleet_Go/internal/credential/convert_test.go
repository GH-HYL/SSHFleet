package credential

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// --convert-secret 的路径来自命令行，原样交给系统：相对路径按当前工作目录解析
// （spec D31），不拼 secret_dir——否则同名文件会撞车。
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

// 绝对路径整条交给文件系统：工作目录里没有这个文件，路径也照原样使用。
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

// 文件不存在时，报错要带出使用者输入的那个路径，否则他不知道是哪次输入打错了。
func TestConvertSecretMissingFileQuotesInput(t *testing.T) {
	t.Chdir(t.TempDir())

	err := ConvertSecret("nope.txt", false)
	if err == nil {
		t.Fatal("文件不存在时应报错")
	}
	if !strings.Contains(err.Error(), "nope.txt") {
		t.Fatalf("报错应含使用者输入的路径，实际：%s", err)
	}
}

// 命令行路径一律原样交给系统：~ 不展开、空白不去掉。两条输入在主目录与当前目录
// 都备好了同名文件，仍必须失败——加工过的路径才会命中它们。
func TestConvertSecretHandsInputToOSUnchanged(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	if err := os.WriteFile(filepath.Join(home, "pass.txt"), []byte("S3cret\n"), 0o644); err != nil {
		t.Fatalf("准备凭据文件失败：%v", err)
	}
	work := t.TempDir()
	t.Chdir(work)
	if err := os.WriteFile(filepath.Join(work, "pass.txt"), []byte("S3cret\n"), 0o644); err != nil {
		t.Fatalf("准备凭据文件失败：%v", err)
	}

	for _, in := range []string{"~/pass.txt", " pass.txt"} {
		if err := ConvertSecret(in, false); err == nil {
			t.Fatalf("%q 应原样交给系统并失败：本程序不展开 ~、不去空白", in)
		}
	}
}
