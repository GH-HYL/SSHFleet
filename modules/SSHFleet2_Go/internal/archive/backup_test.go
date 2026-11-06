package archive

import (
	"os"
	"path/filepath"
	"testing"
)

// 原样复制：内容逐字节同、文件名不变；目录与找不到的路径安静跳过。
func TestBackupCopiesFilesAndSkipsOthers(t *testing.T) {
	dir := t.TempDir()
	assets := filepath.Join(dir, "assets")
	if err := os.MkdirAll(assets, 0o755); err != nil {
		t.Fatalf("建 assets 失败：%v", err)
	}
	script := filepath.Join(dir, "deploy.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\necho hi\n"), 0o755); err != nil {
		t.Fatalf("造脚本失败：%v", err)
	}

	if err := Backup(assets, script, dir, filepath.Join(dir, "nope"), ""); err != nil {
		t.Fatalf("备份不该报错：%v", err)
	}

	got, err := os.ReadFile(filepath.Join(assets, "deploy.sh"))
	if err != nil {
		t.Fatalf("脚本没复制进来：%v", err)
	}
	if string(got) != "#!/bin/sh\necho hi\n" {
		t.Fatalf("复制的内容不对：%q", got)
	}
	entries, err := os.ReadDir(assets)
	if err != nil {
		t.Fatalf("读 assets 失败：%v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("目录与缺失路径不该复制，实际 %d 个条目", len(entries))
	}
}
