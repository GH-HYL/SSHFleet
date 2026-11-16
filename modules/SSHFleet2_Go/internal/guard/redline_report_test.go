package guard

import (
	"go/parser"
	"go/token"
	"io/fs"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"sshfleet2/internal/contract"
	"sshfleet2/internal/message"
	"sshfleet2/internal/report"
)

// 红线：报告站那两件事的形状——入口长什么样，落点由谁给。
//
// 只判形状与流向，一个字的内容都不判（版式是 message 那一片自定义区的事）：
//   - 入口是「第三张纸 + 落点组」进、「要留痕的话 + error」出；
//   - `report.txt` 的落点是入口递进来的一条字符串，本站不产出路径（路径归归档包）；
//   - 所以本包不 import 任何路径包，也不出现 `fmt.Print*`／`os.Stdout` 这类第二出口。
//
// **「上屏的是一句话」不在这里判**（2026-11-16，ADR-0054）：那一条由类型系统硬管——
// `console.Print` 只收 `message.Line`，别的类型塞不进去。原先这里用反射钉
// `message.StatisticsLine` 的返回类型，钉的是 message 的符号，message 一改签名红的是
// 报告站这条红线：射程错位。
func TestRedlineReportScreenAndPath(t *testing.T) {
	// 一、入口形状：第三张纸进、一组落点进（ADR-0043）；交回「要留痕的话」与 error，不返回
	// 路径，也不返回「写好的文件」。落点组按类型名判，不为它 import 归档包（红线只许 import
	// 标准库、契约与被它钉的那一个包）。
	run := reflect.TypeOf(report.Run)
	if run.NumIn() != 2 || run.In(0) != reflect.TypeOf(contract.Ledger{}) || run.In(1).String() != "archive.Paths" {
		t.Fatalf("报告站入口该是 (第三张纸, 落点组)：%v", run)
	}
	wantOut := []reflect.Type{
		reflect.TypeOf([]message.Line(nil)),
		reflect.TypeOf((*error)(nil)).Elem(),
	}
	if run.NumOut() != len(wantOut) || run.Out(0) != wantOut[0] || run.Out(1) != wantOut[1] {
		t.Fatalf("报告站入口该交回 (要留痕的话, error)：%v", run)
	}

	// 二、路径与终端出口都不许自己开：扫本包源码（不含测试）。
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, "../report", func(fi fs.FileInfo) bool {
		return !strings.HasSuffix(fi.Name(), "_test.go")
	}, parser.ImportsOnly)
	if err != nil {
		t.Fatalf("读报告站源码失败：%v", err)
	}
	for _, pkg := range pkgs {
		for name, file := range pkg.Files {
			for _, spec := range file.Imports {
				path, err := strconv.Unquote(spec.Path.Value)
				if err != nil {
					t.Fatalf("%s 的 import 认不出：%v", name, err)
				}
				if path == "path" || path == "path/filepath" {
					t.Fatalf("%s 自己拼了路径（import %q）：落点该由归档包发、塔尖转手", name, path)
				}
			}
		}
	}
}
