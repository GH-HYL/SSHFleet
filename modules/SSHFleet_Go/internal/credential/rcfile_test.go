package credential

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// rc 文件里主密钥行的收敛与读取（2026-09-15 修：写时只替换第一条、读时取第一条，
// 与 shell「最后一条生效」的语义相反，会导致凭据解密报「主密钥不匹配」）。

func keyLines(content string) []string {
	var out []string
	for _, line := range strings.Split(content, "\n") {
		if isKeyExportLine(line) {
			out = append(out, line)
		}
	}
	return out
}

func writeTempRC(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), ".bashrc")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// 重复行收敛：多条主密钥行只留末尾一条，非主密钥行原样保留。
func TestWriteKeyToRCCollapsesDuplicateKeyLines(t *testing.T) {
	path := writeTempRC(t, "alias ll='ls -l'\n"+
		"export SSHFLEET_KEY='OLD-A'\n"+
		"# export SSHFLEET_KEY='COMMENTED'\n"+
		"export EDITOR=vim\n"+
		"export SSHFLEET_KEY='OLD-B'\n")

	if err := writeKeyToRC(path, "NEW-KEY"); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, path)

	if lines := keyLines(got); len(lines) != 1 {
		t.Fatalf("主密钥行应只剩 1 条，实际 %d 条：%q", len(lines), lines)
	}
	if !strings.Contains(got, "export SSHFLEET_KEY='NEW-KEY'") {
		t.Fatalf("新密钥未写入：%q", got)
	}
	for _, keep := range []string{"alias ll='ls -l'", "# export SSHFLEET_KEY='COMMENTED'", "export EDITOR=vim"} {
		if !strings.Contains(got, keep) {
			t.Fatalf("非主密钥行被误删：%q 不在 %q 中", keep, got)
		}
	}
	for _, gone := range []string{"OLD-A", "OLD-B"} {
		if strings.Contains(got, gone) {
			t.Fatalf("旧密钥行未清理：%q 仍在 %q 中", gone, got)
		}
	}

	// 放在末尾：shell 以最后一条赋值生效，读回值必须等于新密钥
	if v := findExportInFile(path); v != "NEW-KEY" {
		t.Fatalf("读回的生效密钥应为 NEW-KEY，实际 %q", v)
	}
	if trimmed := strings.TrimSpace(got); !strings.HasSuffix(trimmed, "export SSHFLEET_KEY='NEW-KEY'") {
		t.Fatalf("主密钥行应在文件末尾（最后一条赋值才生效）：%q", got)
	}
}

// 读取取最后一条：与注释行无关，与 shell 语义一致。
func TestFindExportInFileTakesLastAssignment(t *testing.T) {
	cases := []struct {
		name    string
		content string
		want    string
	}{
		{"多条取最后", "export SSHFLEET_KEY='FIRST'\nexport SSHFLEET_KEY='LAST'\n", "LAST"},
		{"注释行不算数", "export SSHFLEET_KEY='REAL'\n# export SSHFLEET_KEY='COMMENTED'\n", "REAL"},
		{"仅注释行视为无密钥", "# export SSHFLEET_KEY='COMMENTED'\n", ""},
		{"无密钥行", "alias ll='ls -l'\n", ""},
		{"单双引号与无引号", "export SSHFLEET_KEY=\"A\"\nexport SSHFLEET_KEY=B\n", "B"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			path := writeTempRC(t, c.content)
			if got := findExportInFile(path); got != c.want {
				t.Fatalf("findExportInFile 应为 %q，实际 %q", c.want, got)
			}
		})
	}
}

// 幂等：反复写入不堆积空行、不堆积分组注释。
func TestWriteKeyToRCIsIdempotent(t *testing.T) {
	path := writeTempRC(t, "export SSHFLEET_KEY='OLD'\n")
	if err := writeKeyToRC(path, "K1"); err != nil {
		t.Fatal(err)
	}
	first := readFile(t, path)
	if err := writeKeyToRC(path, "K1"); err != nil {
		t.Fatal(err)
	}
	if second := readFile(t, path); second != first {
		t.Fatalf("重复写入内容应一致\n首次：%q\n再次：%q", first, second)
	}
	if strings.Count(first, keyComment) != 1 {
		t.Fatalf("分组注释应只有 1 行：%q", first)
	}
}

// 文件不存在时按新建处理：不额外留空行。
func TestWriteKeyToRCCreatesFileWithoutLeadingBlank(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".zshrc")
	if err := writeKeyToRC(path, "FRESH"); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, path)
	if strings.HasPrefix(got, "\n") {
		t.Fatalf("新建文件不应以空行开头：%q", got)
	}
	if want := keyComment + "\nexport SSHFLEET_KEY='FRESH'\n"; got != want {
		t.Fatalf("新建内容应为 %q，实际 %q", want, got)
	}
	if !hasKeyLine(path) {
		t.Fatal("hasKeyLine 应识别到刚写入的主密钥行")
	}
}

// hasKeyLine 对不存在/无密钥的文件返回 false（persistKey 据此不创建另一侧 rc 文件）。
func TestHasKeyLineFalseCases(t *testing.T) {
	if hasKeyLine(filepath.Join(t.TempDir(), "not-exist")) {
		t.Fatal("不存在的文件不应判为已含主密钥行")
	}
	if hasKeyLine(writeTempRC(t, "# export SSHFLEET_KEY='COMMENTED'\n")) {
		t.Fatal("仅注释行不应判为已含主密钥行")
	}
}
