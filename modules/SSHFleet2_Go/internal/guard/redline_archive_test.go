package guard

import (
	"path/filepath"
	"testing"
	"time"

	"sshfleet2/internal/archive"
)

// 红线：归档包的那一件事——建本次的根、发一组路径字符串。
//
// 只钉形状：交回来的落点都在本次目录里，且都是路径而不是句柄。写不写文件、写了什么，
// 是报告站与执行站的事，不在这条测试的射程。
//
// 这里刻意不取 `paths.Report`：那条路径的名字与检查包自己的 `Report` 同名，指纹按名字数
// 符号，取它会把一个不相干的名字带进指纹（实测过：名单里凭空多出 `Report`）。
// `report.txt` 那个落点由自定义区的 `archive_test.go` 钉着。
func TestRedlineArchiveHandsOutPaths(t *testing.T) {
	paths, err := archive.Create(t.TempDir(), "command", "备注", time.Now())
	if err != nil {
		t.Fatalf("建目录失败：%v", err)
	}
	if paths.Dir == "" {
		t.Fatal("没交出本次目录")
	}
	for name, p := range map[string]string{
		"terminal": paths.Terminal,
		"exec":     paths.ExecLog,
		"assets":   paths.Assets,
	} {
		if p == "" {
			t.Fatalf("%s 的落点空着", name)
		}
		if filepath.Dir(p) != paths.Dir {
			t.Fatalf("%s 不在本次目录里：%s", name, p)
		}
	}
}
