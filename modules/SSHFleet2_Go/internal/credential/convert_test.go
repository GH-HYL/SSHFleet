package credential

import (
	"os"
	"strings"
	"testing"

	"sshfleet2/internal/contract"
)

// 读不到文件时原因要带 `open <路径>:` 前缀（2026-11-10，审计差 3 判「不合理」后回改）。
// 剥掉前缀是别的屏的口径：那边第一行已经说过一遍路径，前缀是重复；这一屏没有，剥了就不知道是哪个文件。
func TestReadConvertibleKeepsOpenPrefix(t *testing.T) {
	_, problem, reason := ReadConvertible("testdata/definitely-not-here.txt")
	if problem != contract.CredFileUnreadable {
		t.Fatalf("读不到应当报 CredFileUnreadable，拿到 %q", problem)
	}
	if !strings.HasPrefix(reason, "open ") {
		t.Fatalf("原因该保留 `open <路径>:` 前缀，实际 %q", reason)
	}
	if !strings.Contains(reason, "definitely-not-here.txt") {
		t.Fatalf("原因里该有那个路径，实际 %q", reason)
	}
}

// 其它几种毛病不该顺带带上系统原因：它们的「毛病」不在系统那一层。
func TestReadConvertibleOtherProblemsCarryNoReason(t *testing.T) {
	dir := t.TempDir()
	empty := writeTemp(t, dir, "empty.txt", "   \n")
	if _, problem, reason := ReadConvertible(empty); problem != contract.CredFileEmpty || reason != "" {
		t.Fatalf("空文件：problem=%q reason=%q", problem, reason)
	}

	binary := writeTemp(t, dir, "bin.txt", "abc\x00def")
	if _, problem, reason := ReadConvertible(binary); problem != contract.CredFileBinary || reason != "" {
		t.Fatalf("二进制：problem=%q reason=%q", problem, reason)
	}

	multi := writeTemp(t, dir, "multi.txt", "line1\nline2\n")
	if _, problem, reason := ReadConvertible(multi); problem != contract.CredFileMultiline || reason != "" {
		t.Fatalf("多行：problem=%q reason=%q", problem, reason)
	}
}

func writeTemp(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := dir + string(os.PathSeparator) + name
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("写样本 %s 失败：%v", name, err)
	}
	return path
}
