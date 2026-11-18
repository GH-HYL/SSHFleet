package guard

import (
	"fmt"
	"strings"
)

// linkDirective 是那条能把别的包的符号绑进来的编译指令。
const linkDirective = "//go:linkname"

// checkLinkEdges 是类二第四条：不经 import 的边也报（ADR-0049）。
//
// 判两样：源码里的 `//go:linkname` 指令、工程里的 `.s` 汇编文件。这两条是唯一能做到
// 「完全不经 import 的跨包绑定」的机制——不禁掉它们，类二那张 import 表只等于「愿意登记的边」。
func checkLinkEdges(m *Module) []Violation {
	var out []Violation
	for _, p := range m.Packages {
		for _, f := range p.Files {
			if !scans(ClassDeps, f) {
				continue
			}
			for _, group := range f.AST.Comments {
				for _, c := range group.List {
					if !strings.HasPrefix(c.Text, linkDirective) {
						continue
					}
					out = append(out, f.violation(ClassDeps, c.Pos(),
						fmt.Sprintf("%s 包里用了 %s 直接绑别的包的符号", p.Short, linkDirective),
						"这条指令建出来的边在 import 表上不存在：依赖图看不见它，反向依赖、跨站取内部都做得成而检查器全绿",
						"删掉它，改成 import ＋ 导出符号；确需跨包取内部，先改 ADR-0032 第四条（红线，单独点头）"))
				}
			}
		}
	}
	for _, rel := range m.AsmFiles {
		out = append(out, violationAt(ClassDeps, rel,
			"工程里有汇编文件 "+rel,
			"汇编可以直接 CALL 任意符号，不经 import——同 `go:linkname`，它造的是一条判据看不见的边",
			"删掉这份汇编；确需用汇编，先改 ADR-0032 第四条（红线，单独点头）"))
	}
	return out
}
