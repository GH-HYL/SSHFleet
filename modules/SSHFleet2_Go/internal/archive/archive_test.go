package archive

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

var fixedNow = time.Date(2026, 11, 2, 9, 30, 5, 0, time.Local)

func TestCreateNamesDirLikeOldTool(t *testing.T) {
	root := t.TempDir()

	got, err := Create(root, "command", "", fixedNow)
	if err != nil {
		t.Fatalf("建目录失败：%v", err)
	}
	wantDir := filepath.Join(root, "2026-11-02_09-30-05_command")
	if got.Dir != wantDir {
		t.Fatalf("目录名不对\n got=%s\nwant=%s", got.Dir, wantDir)
	}
	if fi, err := os.Stat(filepath.Join(wantDir, "assets")); err != nil || !fi.IsDir() {
		t.Fatalf("assets 目录没建出来：%v", err)
	}
}

func TestCreateAppendsSanitizedRemark(t *testing.T) {
	root := t.TempDir()

	got, err := Create(root, "script", "a/b", fixedNow)
	if err != nil {
		t.Fatalf("建目录失败：%v", err)
	}
	want := filepath.Join(root, "2026-11-02_09-30-05_script_a_b")
	if got.Dir != want {
		t.Fatalf("备注里的路径分隔符没换掉\n got=%s\nwant=%s", got.Dir, want)
	}
}

// 备注两头空白不算内容：老版先 TrimSpace 再判空。
func TestCreateIgnoresBlankRemark(t *testing.T) {
	root := t.TempDir()

	got, err := Create(root, "download", "   ", fixedNow)
	if err != nil {
		t.Fatalf("建目录失败：%v", err)
	}
	if filepath.Base(got.Dir) != "2026-11-02_09-30-05_download" {
		t.Fatalf("空白备注不该进目录名：%s", got.Dir)
	}
}

// 落点都在同一个目录里，名字与老版一致。
func TestCreateHandsOutFourPaths(t *testing.T) {
	root := t.TempDir()

	got, err := Create(root, "command", "", fixedNow)
	if err != nil {
		t.Fatalf("建目录失败：%v", err)
	}
	cases := map[string]string{
		"assets":               got.Assets,
		"report.txt":           got.Report,
		"terminal-output.txt":  got.Terminal,
		"SSHFleetExec.log":     got.ExecLog,
		"terminal-output.xlsx": got.OutputXlsx,
		"results.xlsx":         got.ResultsXlsx,
	}
	for base, path := range cases {
		if filepath.Base(path) != base {
			t.Fatalf("%s 的落点不对：%s", base, path)
		}
		if filepath.Dir(path) != got.Dir && filepath.Base(path) == "assets" {
			t.Fatalf("assets 应当在本次目录里：%s", path)
		}
	}
}

// 同一个根下建两次是两个不同目录（时间不同），互不覆盖。
func TestCreateTwiceMakesTwoDirs(t *testing.T) {
	root := t.TempDir()

	a, err := Create(root, "command", "", fixedNow)
	if err != nil {
		t.Fatalf("第一次建目录失败：%v", err)
	}
	b, err := Create(root, "command", "", fixedNow.Add(time.Second))
	if err != nil {
		t.Fatalf("第二次建目录失败：%v", err)
	}
	if a.Dir == b.Dir {
		t.Fatalf("两次建到了同一个目录：%s", a.Dir)
	}
}
