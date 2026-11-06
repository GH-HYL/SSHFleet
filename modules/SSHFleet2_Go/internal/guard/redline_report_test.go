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

// 红线：报告站那两件事的形状——上屏的是一句「话」，落点由别人给，它自己不拼路径、
// 不自开第二个终端出口。
//
// 只判形状与流向，一个字的内容都不判（版式是 message 那一片自定义区的事）：
//   - 屏幕上那一段的类型必须是 `message.Line`——`console.Print` 只收它，别的类型塞不进去；
//   - `report.txt` 的落点是入口递进来的一条字符串，本站不产出路径（路径归归档包）；
//   - 所以本包不 import 任何路径包，也不出现 `fmt.Print*`／`os.Stdout` 这类第二出口。
func TestRedlineReportScreenAndPath(t *testing.T) {
	// 一、上屏那一段是「话」：message.StatisticsLine 交回的必须是 message.Line。
	stats := reflect.TypeOf(message.StatisticsLine)
	if stats.NumOut() != 1 || stats.Out(0) != reflect.TypeOf(message.Line{}) {
		t.Fatalf("统计块交回的不是 message.Line：%v", stats)
	}

	// 二、入口形状：第三张纸进、一组落点进（ADR-0043）；交回「要留痕的话」与 error，不返回
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

	// 三、路径与终端出口都不许自己开：扫本包源码（不含测试）。
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
