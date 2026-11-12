package guard

import (
	"go/ast"
	"go/token"
	"slices"
)

// writerEntry 是 console 交出「往标准输出写字节」的那个入口；writerHome 是唯一许拿它的包。
//
// 别的包拿到它就能绕过 `Print` 直接往屏幕上灌字节——「字只从 message 出」当场失效。所以它
// 按调用点限制，与 os.Exit 只许塔尖调同一种做法（2026-11-02 开发者点头，见 04 规格）。
const (
	writerEntry = "Writer"
	writerHome  = "execute"
)

// printFamily 是「写终端」那一类的点名：fmt 里不指定写入对象、直接落标准输出的三个。
// 带 F 的那几个要指定写入对象，写到内存缓冲不算，所以不在这儿——写到屏幕的那种必然引用
// os.Stdout／os.Stderr，被下面那条抓住。
var printFamily = []string{"Print", "Printf", "Println"}

// messagePkg 是造字的那一站：它的 `Fatal` 是那一屏的唯一入口，正文形状在这里判。
const messagePkg = "message"

// 两个入口各自的正文位置：`Fatal` 是第二个参数，`TraceRefusal` 是第一个。
//
// 为什么两者都判：它们是同一句话的两个形态（屏上与日志），正文同一个——只判屏那一半，
// 掏空日志那一半就没人看得见。
var bodyArgOf = map[string]int{"Fatal": 1, "TraceRefusal": 0}

// CheckTerminal 是类一：谁往终端写字。
//
// 判据（ADR-0032 第四条）：凡往标准输出、标准错误写字即算——fmt.Print 族、内建 println、
// 引用 os.Stdout／os.Stderr，一律算，不看走哪个函数。只有 console 包与 _test.go 文件例外；
// 例外按文件名后缀划，不按目录划——否则打印会搬进一个被生产代码调用的「测试辅助」包。
//
// 第三条判据是 13 号单加的：`console.Writer()` 那条流的调用点只许在执行站。它是同上那条
// 「唯一出口」的放宽——写终端的实体仍只有 console，借 writer 的人只许一个，口子数得出来。
func CheckTerminal(m *Module) []Violation {
	var out []Violation
	for _, p := range m.Packages {
		if p.Short == consolePkg {
			continue
		}
		for _, f := range p.Files {
			if !scans(ClassTerminal, f) {
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
					if why := fatalBodyFault(m, p.Short, f, x); why != "" {
						out = append(out, f.violation(ClassTerminal, x.Pos(), why,
							"那一屏的正文只能由 message 的模板成形：贴进来的句子没有类别，改口吻时找不到它，也不会有人发现两处说法已经漂开（ADR-0047）",
							"把它收成一个类别：站里只交「哪一类、什么参数」，句子向 message 要；系统原文走类别里那一格，别直接当正文"))
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
					if !ok {
						return true
					}
					path := f.ImportPath(id.Name)
					if short, internal := m.Internal(path); internal && short == consolePkg && x.Sel.Name == writerEntry {
						if p.Short != writerHome {
							out = append(out, f.violation(ClassTerminal, x.Pos(),
								"取了 console 的 writer 直接往屏幕写字节",
								"那条流是给进度界面独占屏幕用的，只许执行站拿；别处拿到它就绕开了唯一出口",
								"要说话走 console.Print——它收的是 message 造得出来的那一型"))
						}
						return true
					}
					if path != "os" {
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

// fatalBodyFault 判一次 `message.Fatal`／`TraceRefusal` 调用有没有把来路不明的正文塞进去。
//
// 只判两种形状：正文取自 `.Error()` 调用、正文是当场拼的字符串连接。合法的形状是「message
// 给的正文」——某个类别函数的返回值，或接住它的变量。字面量内容不看、字节不数：那一类判据
// 一碰内容就变成了「这话该怎么说」，而这句话该怎么说只归 message（ADR-0047）。
//
// message 包自己不算：它是造字的地方，正文本来就是它的模板排出来的。
func fatalBodyFault(m *Module, pkg string, f *SourceFile, call *ast.CallExpr) string {
	if pkg == messagePkg {
		return ""
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return ""
	}
	id, ok := sel.X.(*ast.Ident)
	if !ok {
		return ""
	}
	if short, internal := m.Internal(f.ImportPath(id.Name)); !internal || short != messagePkg {
		return ""
	}
	idx, ok := bodyArgOf[sel.Sel.Name]
	if !ok || idx >= len(call.Args) {
		return ""
	}
	return badBodyShape(call.Args[idx])
}

// badBodyShape 看正文那一格是哪种形状；空串表示这一格不违规。
//
// 三种形状算拼出来的：直接取 `.Error()`、把它包一层再交进来、当场用 `+` 拼。
// 交一张纸进来的不算：系统原文走纸里那一格是定好的口子（`CannotStartReason.Origin`），
// 那句话仍由 `message` 成形；这里抓的是「把原文当成句子本身」。
func badBodyShape(expr ast.Expr) string {
	switch x := expr.(type) {
	case *ast.CallExpr:
		if sel, ok := x.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "Error" {
			return "把 err.Error() 直接当成那一屏的正文"
		}
		if hasErrorCall(x) {
			return "把 err.Error() 包了一层再当成那一屏的正文"
		}
	case *ast.BinaryExpr:
		if x.Op == token.ADD {
			return "当场拼了一句字符串塞进那一屏"
		}
	}
	return ""
}

// hasErrorCall 在这个表达式里找「取系统原文」那一下（`.Error()` 调用）。
func hasErrorCall(expr ast.Expr) bool {
	found := false
	ast.Inspect(expr, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if sel, ok := call.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "Error" {
			found = true
			return false
		}
		return true
	})
	return found
}
