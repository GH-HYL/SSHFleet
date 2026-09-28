package output

import (
	"os"
	"path/filepath"
	"testing"

	"sshfleet/internal/cli"
)

// 代填文件（-a 的文件来源）随 assets 原样备份；内联值没有文件、无从备份。
func TestBackupAssetsIncludesAnswerFile(t *testing.T) {
	dir := t.TempDir()
	ans := filepath.Join(dir, "answer.csv")
	if err := os.WriteFile(ans, []byte("1,请选择架构\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ar := &Archive{Dir: filepath.Join(dir, "history_run")}
	a := &cli.Args{Command: "whoami", AnswerFiles: []string{ans}}
	if err := ar.BackupAssets(nil, a); err != nil {
		t.Fatalf("BackupAssets: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(ar.Dir, "assets", "answer.csv"))
	if err != nil {
		t.Fatalf("代填文件应备份进 assets：%v", err)
	}
	if string(got) != "1,请选择架构\n" {
		t.Fatalf("应原样备份，实际：%q", got)
	}
}
