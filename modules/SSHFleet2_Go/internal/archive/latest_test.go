package archive

import (
	"os"
	"path/filepath"
	"testing"
)

// 造一个工作目录（当前目录）与一个带若干次历史目录的历史根。
func latestFixture(t *testing.T, sessions ...string) string {
	t.Helper()
	dir := t.TempDir()
	t.Chdir(dir)
	root := filepath.Join(dir, "history")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatalf("建历史根失败：%v", err)
	}
	for _, s := range sessions {
		if err := os.MkdirAll(filepath.Join(root, s), 0o755); err != nil {
			t.Fatalf("建历史目录失败：%v", err)
		}
	}
	return root
}

// 字典序即时间序：指向最后一个；再跑一次重指到新的那一次。
func TestCreateLatestLinkPointsAtNewest(t *testing.T) {
	root := latestFixture(t, "2026-11-06_09-00-00_command", "2026-11-06_10-00-00_command")
	if _, err := CreateLatestLink(root); err != nil {
		t.Fatalf("建链接失败：%v", err)
	}
	want, err := filepath.Abs(filepath.Join(root, "2026-11-06_10-00-00_command"))
	if err != nil {
		t.Fatalf("算绝对路径失败：%v", err)
	}
	if got, ok := linkTarget(LatestName); !ok || got != want {
		t.Fatalf("指向不对：got=%q want=%q（是链接：%v）", got, want, ok)
	}

	// 又来了一次更新的：重指过去，旧链接先删再建。
	if err := os.MkdirAll(filepath.Join(root, "2026-11-06_11-00-00_command"), 0o755); err != nil {
		t.Fatalf("建历史目录失败：%v", err)
	}
	if _, err := CreateLatestLink(root); err != nil {
		t.Fatalf("重指失败：%v", err)
	}
	want = filepath.Join(root, "2026-11-06_11-00-00_command")
	if abs, err := filepath.Abs(want); err == nil {
		want = abs
	}
	if got, _ := linkTarget(LatestName); got != want {
		t.Fatalf("没有重指：got=%q want=%q", got, want)
	}
}

// 当前目录里是**同名的普通文件**：不删它，交回 skipped 让上面说一句。
func TestCreateLatestLinkSkipsWhenPlainFileExists(t *testing.T) {
	root := latestFixture(t, "2026-11-06_09-00-00_command")
	if err := os.WriteFile(LatestName, []byte("我的文件"), 0o644); err != nil {
		t.Fatalf("造同名文件失败：%v", err)
	}

	skipped, err := CreateLatestLink(root)
	if err != nil {
		t.Fatalf("撞上同名文件不该报错：%v", err)
	}
	if !skipped {
		t.Fatal("应当交回「跳过了」，让上面说一句")
	}
	data, err := os.ReadFile(LatestName)
	if err != nil || string(data) != "我的文件" {
		t.Fatalf("同名文件被动了：%q %v", data, err)
	}
}

// 历史根空着：什么都不做，也不报错。
func TestCreateLatestLinkDoesNothingOnEmptyRoot(t *testing.T) {
	root := latestFixture(t)
	skipped, err := CreateLatestLink(root)
	if err != nil || skipped {
		t.Fatalf("空历史根应当什么都不做：skipped=%v err=%v", skipped, err)
	}
	if _, err := os.Lstat(LatestName); err == nil {
		t.Fatal("空历史根不该建出链接")
	}
}
