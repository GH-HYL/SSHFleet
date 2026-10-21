package credential

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// 固定夹具：一把测试用主密钥，以及它加密 "S3cret-Pa55" 得到的密文（2026-10-21 预先算好写死）。
// 期望的明文是字面量，不从被测代码反推；测试也不读本机真实密钥，任何环境都能跑。
const (
	testMasterKey = "sshfleet-test-master-key-for-convert-secret"
	testCipher    = "ArMz3BYa44K9Ce9StcY9r0lbj/t047K4dwFdqciVdWQIqFtxxEG6aw=="
	testPlain     = "S3cret-Pa55"
)

// writeCred 写一个凭据文件（单行，末尾换行）。
func writeCred(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content+"\n"), 0o644); err != nil {
		t.Fatalf("准备凭据文件失败：%v", err)
	}
}

// readCred 读回凭据文件内容（去掉首尾空白）。
func readCred(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读凭据文件失败：%v", err)
	}
	return strings.TrimSpace(string(b))
}

// --convert-secret 的路径来自命令行，原样交给系统：相对路径按当前工作目录解析
// （spec D31），不拼 secret_dir。改写的必须是当前目录里这一份。
func TestConvertSecretRewritesRelativePathInWorkingDir(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	t.Setenv("SSHFLEET_KEY", testMasterKey)
	writeCred(t, filepath.Join(dir, "pass.txt"), testCipher)

	if err := ConvertSecret("pass.txt", false); err != nil {
		t.Fatalf("当前工作目录下的相对路径应直接命中，实际报错：%v", err)
	}
	if got := readCred(t, filepath.Join(dir, "pass.txt")); got != testPlain {
		t.Fatalf("应把这个文件转成明文 %q，实际内容：%q", testPlain, got)
	}
}

// 绝对路径整条交给文件系统：工作目录里没有这个文件，路径也照原样使用。
func TestConvertSecretTakesAbsolutePathAsIs(t *testing.T) {
	holding := t.TempDir()
	target := filepath.Join(holding, "pass.txt")
	writeCred(t, target, testCipher)
	t.Chdir(t.TempDir())
	t.Setenv("SSHFLEET_KEY", testMasterKey)

	if err := ConvertSecret(target, false); err != nil {
		t.Fatalf("绝对路径应原样命中，实际报错：%v", err)
	}
	if got := readCred(t, target); got != testPlain {
		t.Fatalf("应把这个文件转成明文 %q，实际内容：%q", testPlain, got)
	}
}

// ../ 由系统解析：转的永远是路径指到的那一份，同名邻居一个字节都不许动。
func TestConvertSecretTakesDotDotPathAndLeavesNeighbourAlone(t *testing.T) {
	base := t.TempDir()
	work := filepath.Join(base, "work")
	if err := os.Mkdir(work, 0o755); err != nil {
		t.Fatalf("建工作目录失败：%v", err)
	}
	t.Chdir(work)
	t.Setenv("SSHFLEET_KEY", testMasterKey)
	writeCred(t, filepath.Join(base, "pass.txt"), testCipher)
	writeCred(t, filepath.Join(work, "pass.txt"), testCipher)

	if err := ConvertSecret("../pass.txt", false); err != nil {
		t.Fatalf("../pass.txt 应交由系统解析并命中上级目录，实际报错：%v", err)
	}
	if got := readCred(t, filepath.Join(base, "pass.txt")); got != testPlain {
		t.Fatalf("上级目录那份应被转成明文 %q，实际内容：%q", testPlain, got)
	}
	if got := readCred(t, filepath.Join(work, "pass.txt")); got != testCipher {
		t.Fatalf("当前目录里的同名文件不该被动，实际内容：%q", got)
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
	writeCred(t, filepath.Join(home, "pass.txt"), testPlain)
	work := t.TempDir()
	t.Chdir(work)
	writeCred(t, filepath.Join(work, "pass.txt"), testPlain)

	for _, in := range []string{"~/pass.txt", " pass.txt"} {
		if err := ConvertSecret(in, false); err == nil {
			t.Fatalf("%q 应原样交给系统并失败：本程序不展开 ~、不去空白", in)
		}
	}
}
