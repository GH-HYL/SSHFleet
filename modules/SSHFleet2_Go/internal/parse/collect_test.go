package parse

// 上传源采集（建站 05 的 06 号单）：五种条目、过滤后不剩一个、相对路径、总大小与建议并发、
// 根下一层的条目。
//
// 对着临时目录造真条目：软链接、FIFO 与不可读文件都不是「传个结构体进去」验得出来的。

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sshfleet2/internal/contract"
	"sshfleet2/internal/message"
)

// collectPlan 造一张只带上传任务的纸，跑一遍 collect。
func collectPlan(t *testing.T, source string) (*contract.Plan, *contract.CannotStart) {
	t.Helper()
	cfg := &settings{}
	cfg.upload.SmallFile = 1024
	cfg.upload.LargeFile = 1024 * 1024
	cfg.upload.MediumParallel = 4

	p := &contract.Plan{
		Mode: contract.ModeUpload,
		Task: contract.Task{Upload: &contract.UploadTask{Source: source}, Path: "/tmp/"},
		Nodes: []contract.Node{
			{IP: "10.0.0.1"}, {IP: "10.0.0.2"}, {IP: "10.0.0.3"},
			{IP: "10.0.0.4"}, {IP: "10.0.0.5"}, {IP: "10.0.0.6"},
		},
	}
	return p, collect(p, cfg)
}

// 普通文件与目录：目录递归、相对路径按 POSIX 斜杠、总大小求和。
func TestCollectFilesAndDirs(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "dist")
	if err := os.MkdirAll(filepath.Join(root, "sub"), 0o755); err != nil {
		t.Fatalf("造目录失败：%v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("12345"), 0o644); err != nil {
		t.Fatalf("造文件失败：%v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "sub", "b.txt"), []byte("123"), 0o644); err != nil {
		t.Fatalf("造文件失败：%v", err)
	}

	p, fail := collectPlan(t, root)
	if fail != nil {
		t.Fatalf("不该报错：%v", message.CannotStartText(*fail))
	}
	if len(p.Task.Upload.Files) != 2 {
		t.Fatalf("应当采到两个文件：%+v", p.Task.Upload.Files)
	}
	want := map[string]bool{"a.txt": true, "sub/b.txt": true}
	for _, f := range p.Task.Upload.Files {
		if !want[f.Rel] {
			t.Errorf("相对路径不对：%q（不含上传根那一层）", f.Rel)
		}
	}
	if p.Task.Upload.TotalBytes != 8 {
		t.Fatalf("总大小应当是 8，实际 %d", p.Task.Upload.TotalBytes)
	}
	if !p.Task.Upload.RootIsDir {
		t.Fatal("根是目录，这一格应当为真")
	}
}

// 单文件：根就是那个文件，相对路径是文件名。
func TestCollectSingleFile(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "app.bin")
	if err := os.WriteFile(file, []byte("1234567890"), 0o644); err != nil {
		t.Fatalf("造文件失败：%v", err)
	}
	p, fail := collectPlan(t, file)
	if fail != nil {
		t.Fatalf("不该报错：%v", message.CannotStartText(*fail))
	}
	if len(p.Task.Upload.Files) != 1 || p.Task.Upload.Files[0].Rel != "app.bin" {
		t.Fatalf("单文件的相对路径应当是文件名：%+v", p.Task.Upload.Files)
	}
	if p.Task.Upload.RootIsDir {
		t.Fatal("单文件上传时根不是目录")
	}
}

// 软链接一律过滤、不报错、留名；被过滤的链接也要进「根下一层」，确认屏要列得出来。
func TestCollectSkipsSymlinks(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "dist")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatalf("造目录失败：%v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "real.txt"), []byte("x"), 0o644); err != nil {
		t.Fatalf("造文件失败：%v", err)
	}
	if err := os.Symlink(filepath.Join(root, "real.txt"), filepath.Join(root, "link.txt")); err != nil {
		t.Fatalf("造软链接失败：%v", err)
	}

	p, fail := collectPlan(t, root)
	if fail != nil {
		t.Fatalf("软链接不该报错：%v", message.CannotStartText(*fail))
	}
	if len(p.Task.Upload.Files) != 1 {
		t.Fatalf("链接不该进可传文件表：%+v", p.Task.Upload.Files)
	}
	if len(p.Task.Upload.Skipped) != 1 || p.Task.Upload.Skipped[0] != "link.txt" {
		t.Fatalf("被过滤的链接要留名：%+v", p.Task.Upload.Skipped)
	}
	link := false
	for _, e := range p.Task.Upload.TopEntries {
		if e.Name == "link.txt" {
			link = true
		}
	}
	if !link {
		t.Fatalf("根下一层要含被过滤的链接：%+v", p.Task.Upload.TopEntries)
	}
}

// 上传根本身是软链接：没有可传目标，报错。
func TestCollectRootSymlinkFails(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "real.txt")
	if err := os.WriteFile(target, []byte("x"), 0o644); err != nil {
		t.Fatalf("造文件失败：%v", err)
	}
	link := filepath.Join(dir, "link.txt")
	if err := os.Symlink(target, link); err != nil {
		t.Fatalf("造软链接失败：%v", err)
	}
	if _, fail := collectPlan(t, link); fail == nil {
		t.Fatal("根本身是软链接时应当报错")
	}
}

// 过滤后一个可传文件都不剩：报错，说清滤掉了几个、都是谁。
func TestCollectAllFilteredFails(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "dist")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatalf("造目录失败：%v", err)
	}
	real := filepath.Join(dir, "outside.txt")
	if err := os.WriteFile(real, []byte("x"), 0o644); err != nil {
		t.Fatalf("造文件失败：%v", err)
	}
	if err := os.Symlink(real, filepath.Join(root, "only.txt")); err != nil {
		t.Fatalf("造软链接失败：%v", err)
	}
	_, fail := collectPlan(t, root)
	if fail == nil {
		t.Fatal("一个可传文件都不剩时应当报错")
	}
	if want := "已过滤 1 个：only.txt"; !strings.Contains(message.CannotStartText(*fail), want) {
		t.Fatalf("报错要说清滤掉了谁：%s", message.CannotStartText(*fail))
	}
}

// 不可读文件：现在就报，不等传到一半。
func TestCollectUnreadableFileFails(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root 不受文件权限限制，这一条验不出来")
	}
	dir := t.TempDir()
	root := filepath.Join(dir, "dist")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatalf("造目录失败：%v", err)
	}
	secret := filepath.Join(root, "secret.txt")
	if err := os.WriteFile(secret, []byte("x"), 0o644); err != nil {
		t.Fatalf("造文件失败：%v", err)
	}
	if err := os.Chmod(secret, 0o000); err != nil {
		t.Fatalf("改权限失败：%v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(secret, 0o644) })

	_, fail := collectPlan(t, root)
	if fail == nil {
		t.Fatal("不可读的文件应当当场报错")
	}
	if say := message.CannotStartText(*fail); !strings.Contains(say, "文件不可读") {
		t.Fatalf("话里要说清是不可读：%s", say)
	}
}

// 空子目录也要进「根下一层」：文件表里没有它，靠反推就会漏。
func TestCollectTopEntriesIncludeEmptyDir(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "dist")
	if err := os.MkdirAll(filepath.Join(root, "empty"), 0o755); err != nil {
		t.Fatalf("造目录失败：%v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("x"), 0o644); err != nil {
		t.Fatalf("造文件失败：%v", err)
	}
	p, fail := collectPlan(t, root)
	if fail != nil {
		t.Fatalf("不该报错：%v", message.CannotStartText(*fail))
	}
	found := false
	for _, e := range p.Task.Upload.TopEntries {
		if e.Name == "empty" && e.IsDir {
			found = true
		}
	}
	if !found {
		t.Fatalf("空子目录要进根下一层：%+v", p.Task.Upload.TopEntries)
	}
}

// 建议并发：按配置阈值给，并按节点数封顶。
func TestCollectSuggestsConcurrency(t *testing.T) {
	mk := func(size int) string {
		dir := t.TempDir()
		root := filepath.Join(dir, "dist")
		if err := os.Mkdir(root, 0o755); err != nil {
			t.Fatalf("造目录失败：%v", err)
		}
		if err := os.WriteFile(filepath.Join(root, "a.bin"), make([]byte, size), 0o644); err != nil {
			t.Fatalf("造文件失败：%v", err)
		}
		return root
	}

	// 小于 small_file（1024）：不限制，0。
	p, fail := collectPlan(t, mk(10))
	if fail != nil {
		t.Fatalf("不该报错：%v", message.CannotStartText(*fail))
	}
	if p.Run.ConcurrencyAdvice != 0 {
		t.Fatalf("小文件不该有建议，实际 %d", p.Run.ConcurrencyAdvice)
	}

	// 中间那段：取 medium_parallel（4），节点 6 台不封顶。
	if p, fail = collectPlan(t, mk(4096)); fail != nil {
		t.Fatalf("不该报错：%v", message.CannotStartText(*fail))
	}
	if p.Run.ConcurrencyAdvice != 4 {
		t.Fatalf("中等大小应当建议 4，实际 %d", p.Run.ConcurrencyAdvice)
	}

	// 大于 large_file（1MiB）：一次一个。
	if p, fail = collectPlan(t, mk(2*1024*1024)); fail != nil {
		t.Fatalf("不该报错：%v", message.CannotStartText(*fail))
	}
	if p.Run.ConcurrencyAdvice != 1 {
		t.Fatalf("大文件应当建议 1，实际 %d", p.Run.ConcurrencyAdvice)
	}
}

// 建议并发按节点数封顶：机器比建议少时，建议不可能比机器还多。
func TestCollectSuggestionCappedByNodes(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "dist")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatalf("造目录失败：%v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "a.bin"), make([]byte, 4096), 0o644); err != nil {
		t.Fatalf("造文件失败：%v", err)
	}

	cfg := &settings{}
	cfg.upload.SmallFile = 1024
	cfg.upload.LargeFile = 1024 * 1024
	cfg.upload.MediumParallel = 4

	p := &contract.Plan{
		Mode:  contract.ModeUpload,
		Task:  contract.Task{Upload: &contract.UploadTask{Source: root}, Path: "/tmp/"},
		Nodes: []contract.Node{{IP: "10.0.0.1"}, {IP: "10.0.0.2"}},
	}
	if fail := collect(p, cfg); fail != nil {
		t.Fatalf("不该报错：%v", message.CannotStartText(*fail))
	}
	if p.Run.ConcurrencyAdvice != 2 {
		t.Fatalf("建议应当按节点数封顶到 2，实际 %d", p.Run.ConcurrencyAdvice)
	}
}
