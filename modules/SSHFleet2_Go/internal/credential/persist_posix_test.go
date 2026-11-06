//go:build !windows

package credential

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// 写持久值：当前 rc 收敛成末尾唯一一条，另一侧已存在的跟着改成同一把，不存在的不新建。
func TestPersistKeyConvergesRCFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("SHELL", "/bin/bash")

	bashrc := filepath.Join(home, ".bashrc")
	zshrc := filepath.Join(home, ".zshrc")
	write(t, bashrc, "# 我的配置\nexport PATH=/usr/bin\n"+
		"# SSHFleet 主密钥\nexport SSHFLEET_KEY='OLD-1'\n"+
		"export SSHFLEET_KEY='OLD-2'\n")
	write(t, zshrc, "# SSHFleet 主密钥\nexport SSHFLEET_KEY='ZSH-OLD'\n")

	res, err := persistKey("NEWKEY")
	if err != nil {
		t.Fatalf("写持久值失败：%v", err)
	}
	if res.Where != bashrc {
		t.Fatalf("应当写当前 shell 的 rc：want %s，got %s", bashrc, res.Where)
	}
	if res.OtherUnified != zshrc {
		t.Fatalf("另一侧应当被统一：want %s，got %s", zshrc, res.OtherUnified)
	}

	if got, want := readFile(t, bashrc), "# 我的配置\nexport PATH=/usr/bin\n\n# SSHFleet 主密钥\nexport SSHFLEET_KEY='NEWKEY'\n"; got != want {
		t.Fatalf("bashrc 收敛结果不对：\n got %q\nwant %q", got, want)
	}
	if got, want := readFile(t, zshrc), "# SSHFleet 主密钥\nexport SSHFLEET_KEY='NEWKEY'\n"; got != want {
		t.Fatalf("zshrc 应当改成同一把：\n got %q\nwant %q", got, want)
	}
}

// 另一侧不存在时不新建它：使用者的主目录里不该凭空多一个 rc 文件。
func TestPersistKeyDoesNotCreateOtherSide(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("SHELL", "/bin/bash")

	if _, err := persistKey("NEWKEY"); err != nil {
		t.Fatalf("写持久值失败：%v", err)
	}
	if _, err := os.Stat(filepath.Join(home, ".zshrc")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf(".zshrc 不该被新建，got err=%v", err)
	}
	// 当前 shell 那一侧本来没有，也要建出来——否则空主目录上 --gen-key 等于没写成。
	if got, want := readFile(t, filepath.Join(home, ".bashrc")), "# SSHFleet 主密钥\nexport SSHFLEET_KEY='NEWKEY'\n"; got != want {
		t.Fatalf("bashrc 应当新建并写入：\n got %q\nwant %q", got, want)
	}
}
