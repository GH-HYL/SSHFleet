package cli

import (
	"os"
	"path/filepath"
	"testing"
)

// 取值来源判定（-f 与 -a 共用）：路径存在即文件，否则当内联文本。
// 判定不看后缀名——`nodes` 也叫文件。

func TestResolveSourceFile(t *testing.T) {
	file := filepath.Join(t.TempDir(), "nodes") // 故意不带 .csv 后缀
	if err := os.WriteFile(file, []byte("127.0.0.1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if src := resolveSource(file); !src.IsFile || src.Path != file {
		t.Fatalf("已存在的路径应判成文件来源，实际：%+v", src)
	}
}

func TestResolveSourceInline(t *testing.T) {
	raw := `127.0.0.1,22,root,pw\n127.0.0.2,22,root,pw`
	src := resolveSource(raw)
	if src.IsFile {
		t.Fatalf("路径不存在时应判成内联文本，实际：%+v", src)
	}
	if src.Raw != raw {
		t.Fatalf("内联文本要原样留着，实际：%q", src.Raw)
	}
}

// -f 的内联值必须原样进 Args：路径规范化会把 `\n` 弄成 `/n`，
// 那会让多行清单静默缩成一行、字段整体错位。
func TestParseKeepsInlineListRaw(t *testing.T) {
	raw := `127.0.0.1,22,root,pw\n127.0.0.2,22,root,pw`
	a, err := Parse(helpTestCfg(), "9.9.9", []string{"-f", raw, "-c", "uptime"})
	if err != nil {
		t.Fatalf("内联清单应解析通过：%v", err)
	}
	if !a.FIsInline {
		t.Fatal("路径不存在时应标成内联清单")
	}
	if a.CsvFile != raw {
		t.Fatalf("内联清单应原样留着，实际：%q", a.CsvFile)
	}
}

// 内联值是 CSV 内容、不是路径：它中间出现空格是正常的，不该被路径规则拦下。
func TestParseAllowsInlineListWithSpace(t *testing.T) {
	a, err := Parse(helpTestCfg(), "9.9.9", []string{"-f", "127.0.0.1,22,root,p w", "-c", "uptime"})
	if err != nil {
		t.Fatalf("内联清单里的空格不该报错：%v", err)
	}
	if !a.FIsInline {
		t.Fatal("应标成内联清单")
	}
}

// 文件形态照旧走路径规则：文件存在就认文件，与后缀名无关。
func TestParseListFileIsNotInline(t *testing.T) {
	file := filepath.Join(t.TempDir(), "nodes.txt") // 后缀不是 .csv 也一样
	if err := os.WriteFile(file, []byte("127.0.0.1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	a, err := Parse(helpTestCfg(), "9.9.9", []string{"-f", file, "-c", "uptime"})
	if err != nil {
		t.Fatalf("正常路径应解析通过：%v", err)
	}
	if a.FIsInline {
		t.Fatal("已存在的文件不该标成内联清单")
	}
}

// 文件形态仍受空格禁令约束（内联形态不受——见上一条）。
func TestParseRejectsListPathWithSpace(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "my dir")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(dir, "nodes.csv")
	if err := os.WriteFile(file, []byte("127.0.0.1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Parse(helpTestCfg(), "9.9.9", []string{"-f", file, "-c", "uptime"}); err == nil {
		t.Fatal("路径含空格应报错")
	}
}
