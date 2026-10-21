package cli

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// 取值来源判定（-f 与 -a 共用）：路径存在即文件，否则当一条内联记录。
// 判定不看后缀名——`nodes` 也叫文件；也不加工路径——`\` 不换成 `/`、
// 不去空白、不拼前缀（ADR-0023）。

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
	raw := `127.0.0.1,22,root,D:\new\pw.bin`
	src := resolveSource(raw)
	if src.IsFile {
		t.Fatalf("路径不存在时应判成内联文本，实际：%+v", src)
	}
	if src.Raw != raw {
		t.Fatalf("内联文本要原样留着，实际：%q", src.Raw)
	}
}

// -f 的内联值必须原样进 Args：命令行给的值不由程序加工，反斜杠与 `;;` 都不动。
func TestParseKeepsInlineListRaw(t *testing.T) {
	raw := `127.0.0.1,22,root,D:\new\pw.bin`
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

// 文件形态不再受空格禁令约束（内联形态本来就不受——见上一条）。
//
// 2026-10-15 口径收窄：空格检查只作用于验不了存在性的**远端**参数。`-f` 是本地文件，
// 存在性验证能给出准确答案（「文件不存在：X」），拿空格去拦反而把更准的报错截胡。
// 带空格的路径只要能找到就正常当文件用。
func TestParseAcceptsListPathWithSpace(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "my dir")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(dir, "nodes.csv")
	if err := os.WriteFile(file, []byte("127.0.0.1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	a, err := Parse(helpTestCfg(), "9.9.9", []string{"-f", file, "-c", "uptime"})
	if err != nil {
		t.Fatalf("路径含空格的已存在文件应正常接受：%v", err)
	}
	if a.FIsInline {
		t.Fatal("已存在的文件（哪怕路径含空格）不该标成内联清单")
	}
}

// -f 的取值判定：单条裸 IP、单条四列、首字段带引号，都认得出来是一条节点。
func TestCheckFInlineRecordsPass(t *testing.T) {
	for _, val := range []string{
		"10.0.0.1",
		"10.0.0.1,22,root,pw",
		`"10.0.0.1",22,root,pw`, // Excel 导出的单行 CSV
	} {
		t.Run(val, func(t *testing.T) {
			a, err := Parse(helpTestCfg(), "9.9.9", []string{"-f", val, "-c", "uptime"})
			if err != nil {
				t.Fatalf("Parse 不该报错：%v", err)
			}
			if !a.FIsInline {
				t.Fatal("不是文件，应标成内联")
			}
			if err := CheckArguments(a); err != nil {
				t.Fatalf("-f %q 应通过：%v", val, err)
			}
		})
	}
}

// `;;` 不再是分隔符：写成两台的那串整体按一条处理，报「文件不存在」，
// 且不出现任何只针对 `;;` 的说明（ADR-0023）。
func TestCheckFSemicolonPairIsOneRecord(t *testing.T) {
	a, err := Parse(helpTestCfg(), "9.9.9", []string{"-f", "10.0.0.1;;10.0.0.2", "-c", "uptime"})
	if err != nil {
		t.Fatalf("Parse 不该报错：%v", err)
	}
	err = CheckArguments(a)
	if err == nil {
		t.Fatal("既不是文件也不像一行节点信息，应报错")
	}
	if !strings.Contains(err.Error(), "文件不存在") {
		t.Fatalf("应与路径打错同形（文件不存在），实际：%v", err)
	}
	if strings.Contains(err.Error(), "内联只写一条") {
		t.Fatalf("`;;` 不该触发专门提示：%v", err)
	}
}

// 内联值里出现换行：直接拒绝，并指向写文件。
func TestCheckFRejectsNewline(t *testing.T) {
	cases := map[string]string{
		"裸 IP 两条": "10.0.0.1\n10.0.0.2",
		"完整列两条":   "10.0.0.1,22,root,pw\n10.0.0.2,22,root,pw",
		"CRLF":    "10.0.0.1,22,root,pw\r\n10.0.0.2,22,root,pw",
		"孤立 CR":   "10.0.0.1\r10.0.0.2",
	}
	for name, val := range cases {
		t.Run(name, func(t *testing.T) {
			a, err := Parse(helpTestCfg(), "9.9.9", []string{"-f", val, "-c", "uptime"})
			if err != nil {
				t.Fatalf("Parse 不该报错：%v", err)
			}
			err = CheckArguments(a)
			if err == nil {
				t.Fatal("含换行的内联值应被拒绝")
			}
			if !strings.Contains(err.Error(), "不能有换行") || !strings.Contains(err.Error(), "CSV 文件") {
				t.Fatalf("报错应说清不能有换行、并指向文件，实际：%v", err)
			}
		})
	}
}

// 命令行给的路径不加工：Windows 风格分隔符在 Linux 上不会被换成 `/` 后再试。
func TestParseDoesNotRewriteWindowsStylePath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows 的 `\\` 本来就能被系统解析，这条钉的是「不做字符串加工」")
	}
	dir := t.TempDir()
	sub := filepath.Join(dir, "nodes")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "a.csv"), []byte("127.0.0.1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	winStyle := dir + `\nodes\a.csv`
	a, err := Parse(helpTestCfg(), "9.9.9", []string{"-f", winStyle, "-c", "uptime"})
	if err != nil {
		t.Fatalf("Parse 不该报错：%v", err)
	}
	if !a.FIsInline {
		t.Fatal("原值找不到就按内联处理，不该被改写成 / 之后认成文件")
	}
	if err := CheckArguments(a); err == nil || !strings.Contains(err.Error(), "文件不存在") {
		t.Fatalf("应报文件不存在，实际：%v", err)
	}
}

// 路径也不去首尾空白：带尾随空格的值不会被整理成前面那个文件。
func TestParseDoesNotTrimPath(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "nodes.csv")
	if err := os.WriteFile(file, []byte("127.0.0.1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	a, err := Parse(helpTestCfg(), "9.9.9", []string{"-f", file + " ", "-c", "uptime"})
	if err != nil {
		t.Fatalf("Parse 不该报错：%v", err)
	}
	if !a.FIsInline {
		t.Fatal("带尾随空格的原值找不到就按内联处理，不该被去掉空白后认成文件")
	}
	if err := CheckArguments(a); err == nil || !strings.Contains(err.Error(), "文件不存在") {
		t.Fatalf("应报文件不存在，实际：%v", err)
	}
}
