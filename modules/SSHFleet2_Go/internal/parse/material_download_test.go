package parse

// 下载模式的 `-p`：本机目录要已建好、是目录、而且是空的。
//
// 走真入口 `Run`：这三支都是材料自检，失败以「延后的起不来」交出来（第五个返回值），
// 验的就是「递一张 argv、收一条起不来」这件事。

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sshfleet2/internal/contract"
	"sshfleet2/internal/message"
)

func TestDownloadPathMustBeEmpty(t *testing.T) {
	dir := writeTempConfig(t)
	manifest := writeFile(t, dir, "ok.csv", "10.0.0.1,22,root,pw\n")

	// 空目录：放行，连一条延后的起不来都没有。
	empty := t.TempDir()
	if _, _, _, _, fail := Run([]string{"sshfleet2", "-f", manifest, "-d", "/srv/data", "-p", empty}, "test"); fail != nil {
		t.Fatalf("空目录不该被拦：%v", message.CannotStartBody(*fail).Text())
	}

	// 非空：拦住，交的是「起不来」，不是「拒绝」。
	used := t.TempDir()
	if err := os.WriteFile(filepath.Join(used, "old.txt"), []byte("old"), 0o644); err != nil {
		t.Fatalf("造一个占位文件失败：%v", err)
	}
	_, _, _, _, fail := Run([]string{"sshfleet2", "-f", manifest, "-d", "/srv/data", "-p", used}, "test")
	if fail == nil {
		t.Fatal("非空的本地目录该被拦下")
	}
	if fail.Reason.Kind != contract.CannotStartDownloadPathNotEmpty {
		t.Fatalf("该归「不是空目录」那一支，实际 %q", fail.Reason.Kind)
	}
	if fail.Reason.Place != used {
		t.Fatalf("该带上那个目录，实际 %q", fail.Reason.Place)
	}

	// 文案三样齐：哪儿错、为什么、怎么改。
	text := message.CannotStartBody(*fail).Text()
	for _, want := range []string{used, "空目录", "清空"} {
		if !strings.Contains(text, want) {
			t.Fatalf("文案里该有 %q，实际 %q", want, text)
		}
	}
}
