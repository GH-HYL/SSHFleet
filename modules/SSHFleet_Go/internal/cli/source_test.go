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
// 不去空白、不拼前缀（ADR-0023）。从命令行入口看 FIsInline 这个结果。

func TestSourceFileWithoutCsvSuffix(t *testing.T) {
	file := filepath.Join(t.TempDir(), "nodes") // 故意不带 .csv 后缀
	if err := os.WriteFile(file, []byte("127.0.0.1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	a, err := Parse(helpTestCfg(), "9.9.9", []string{"-f", file, "-c", "uptime"})
	if err != nil {
		t.Fatalf("Parse 不该报错：%v", err)
	}
	if err := CheckArguments(a); err != nil {
		t.Fatalf("已存在的文件应通过合规检查：%v", err)
	}
	if a.FIsInline {
		t.Fatal("已存在的路径应判成文件来源，不看后缀名")
	}
	if a.CsvFile != file {
		t.Fatalf("路径应原样留着，实际：%q", a.CsvFile)
	}
}

func TestSourceInlineKeepsRawValue(t *testing.T) {
	raw := `127.0.0.1,22,root,D:\new\pw.bin`
	a, err := Parse(helpTestCfg(), "9.9.9", []string{"-f", raw, "-c", "uptime"})
	if err != nil {
		t.Fatalf("Parse 不该报错：%v", err)
	}
	if err := CheckArguments(a); err != nil {
		t.Fatalf("一条内联记录应通过合规检查：%v", err)
	}
	if !a.FIsInline {
		t.Fatal("路径不存在时应判成内联记录")
	}
	if a.CsvFile != raw {
		t.Fatalf("内联值要原样留着，实际：%q", a.CsvFile)
	}
}

// -f 的内联值必须原样进 Args：命令行给的值不由程序加工，反斜杠与 `;;` 都不动。
func TestParseKeepsInlineListRaw(t *testing.T) {
	raw := `127.0.0.1,22,root,D:\new\pw.bin`
	a, err := Parse(helpTestCfg(), "9.9.9", []string{"-f", raw, "-c", "uptime"})
	if err != nil {
		t.Fatalf("内联清单应解析通过：%v", err)
	}
	if err := CheckArguments(a); err != nil {
		t.Fatalf("合规检查应通过：%v", err)
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
	if err := CheckArguments(a); err != nil {
		t.Fatalf("合规检查应通过：%v", err)
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
	if err := CheckArguments(a); err != nil {
		t.Fatalf("合规检查应通过：%v", err)
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
	if err := CheckArguments(a); err != nil {
		t.Fatalf("合规检查应通过：%v", err)
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
			if err := CheckArguments(a); err != nil {
				t.Fatalf("-f %q 应通过：%v", val, err)
			}
			if !a.FIsInline {
				t.Fatal("不是文件，应标成内联")
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
	if !strings.Contains(err.Error(), "参数指定的文件读不到") || !strings.Contains(err.Error(), "原因：") {
		t.Fatalf("应与路径打错同形（读不到 + 系统原文），实际：%v", err)
	}
	if strings.Contains(err.Error(), "内联只写一条") {
		t.Fatalf("`;;` 不该触发专门提示：%v", err)
	}
}

// 内联值里出现换行：直接拒绝，原因只说一件（不能有换行），去路放在提示行。
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
			want := "-f 的内联值里不能有换行：内联只写一条\n提示：把节点写进 CSV 文件，再用 -f 指向它"
			if err.Error() != want {
				t.Fatalf("文案应为 %q，实际：%q", want, err.Error())
			}
		})
	}
}

// 命令行给的路径不加工：Windows 风格分隔符在 Linux 上不会被换成 `/` 后再试。
//
// 断言落在**报错文案里出现的是原值**上。「出错之后 FIsInline 仍为真」那种断言不成立：
// 合规检查在 `-f` 取值失败的那一刻就返回了，FIsInline 只在成功路径上回填。
// 钉住原值才是这条用例真正想说的话。
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
	cerr := CheckArguments(a)
	if cerr == nil {
		t.Fatalf("原值在 Linux 上找不到，应报错（报不出就说明被改写成 / 之后又找到了）")
	}
	if !strings.Contains(cerr.Error(), winStyle) {
		t.Fatalf("报错文案里应是原值 %q，实际：%v", winStyle, cerr)
	}
}

// 路径不去空白：带首尾空白的值原样交给系统，不会被整理成前面那个文件。
//
// 用**前导**空格钉：Windows 的 os.Stat 会把尾随空格裁掉再找，尾随空格那条在这里
// 验不出「程序有没有加工原值」（实测会命中 nodes.csv）。前导空格两边系统都不裁剪。
func TestParseDoesNotTrimPath(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "nodes.csv")
	if err := os.WriteFile(file, []byte("127.0.0.1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	a, err := Parse(helpTestCfg(), "9.9.9", []string{"-f", " " + file, "-c", "uptime"})
	if err != nil {
		t.Fatalf("Parse 不该报错：%v", err)
	}
	// 判定在合规检查阶段做：它报「读不到」就把**原值**（带前导空格的那个）回显在消息里，
	// 说明没被去空白后认成文件。
	if err := CheckArguments(a); err == nil || !strings.Contains(err.Error(), " "+file) {
		t.Fatalf("应报原值读不到并把原值回显出来，实际：%v", err)
	}
}

// `-f` 与 `-a` 是同一套判定：同一个不存在的路径，两边报同一句话（只说参数名不同）。
// 从命令行入口看：两边都走一处判定，报错才会同形。
func TestSameGateForFAndA(t *testing.T) {
	const gone = "nosuch-file.csv"
	for _, tc := range []struct{ flag, raw string }{
		{"-f", gone},
		{"-a", gone},
	} {
		t.Run(tc.flag, func(t *testing.T) {
			a, err := Parse(helpTestCfg(), "9.9.9", []string{"-c", "uptime", tc.flag, tc.raw})
			if err != nil {
				t.Fatalf("Parse 不该报错：%v", err)
			}
			err = CheckArguments(a)
			if err == nil {
				t.Fatalf("%s %s 应报错", tc.flag, tc.raw)
			}
			want := tc.flag + " 参数指定的文件读不到：" + tc.raw
			if !strings.Contains(err.Error(), want) {
				t.Fatalf("应与另一侧同形（含 %q），实际：%v", want, err)
			}
		})
	}
}
