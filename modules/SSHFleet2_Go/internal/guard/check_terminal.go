package guard

import (
	"go/ast"
	"slices"
)

// printFamily 是「写终端」那一类的点名：fmt 里不指定写入对象、直接落标准输出的三个。
// 带 F 的那几个要指定写入对象，写到内存缓冲不算，所以不在这儿——写到屏幕的那种必然引用
// os.Stdout／os.Stderr，被下面那条抓住。
var printFamily = []string{"Print", "Printf", "Println"}

// CheckTerminal 是类一：谁往终端写字。
//
// 判据（ADR-0032 第四条）：凡往标准输出、标准错误写字即算——fmt.Print 族、内建 println、
// 引用 os.Stdout／os.Stderr，一律算，不看走哪个函数。只有 console 包与 _test.go 文件例外；
// 例外按文件名后缀划，不按目录划——否则打印会搬进一个被生产代码调用的「测试辅助」包。
func CheckTerminal(m *Module) []Violation {
	var out []Violation
	for _, p := range m.Packages {
		if p.Short == consolePkg {
			continue
		}
		for _, f := range p.Files {
			if f.IsTest {
				continue
			}
			ast.Inspect(f.AST, func(n ast.Node) bool {
				switch x := n.(type) {
				case *ast.CallExpr:
					if id, ok := x.Fun.(*ast.Ident); ok && id.Name == "println" {
						out = append(out, f.violation(ClassTerminal, id.Pos(),
							"用了内建 println 往标准错误写字",
							"往终端写字的动作只许出现在 console 包，内建 println 绕过它直接落标准错误",
							"把这句话交给 console——它收的是 message 造出来的那一型"))
						return true
					}
					sel, ok := x.Fun.(*ast.SelectorExpr)
					if !ok {
						return true
					}
					id, ok := sel.X.(*ast.Ident)
					if !ok || f.ImportPath(id.Name) != "fmt" || !slices.Contains(printFamily, sel.Sel.Name) {
						return true
					}
					out = append(out, f.violation(ClassTerminal, sel.Pos(),
						"调了 fmt."+sel.Sel.Name+" 直接往标准输出写字",
						"往终端的出口只有 console 包一个；别处自己打印，字就不从 message 那一处成形",
						"把要显示的事实交给 console——它收的是 message 造得出来的那一型"))
				case *ast.SelectorExpr:
					id, ok := x.X.(*ast.Ident)
					if !ok || f.ImportPath(id.Name) != "os" {
						return true
					}
					if x.Sel.Name != "Stdout" && x.Sel.Name != "Stderr" {
						return true
					}
					out = append(out, f.violation(ClassTerminal, x.Pos(),
						"直接引用了 os."+x.Sel.Name,
						"标准输出与标准错误只许 console 包碰；别处拿到它就是绕开了唯一出口",
						"要写字走 console，要读输入走 ask"))
				}
				return true
			})
		}
	}
	return out
}
