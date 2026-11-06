package guard

import (
	"go/ast"
	"go/token"
	"strconv"
	"strings"
)

// maskHome 是遮的构造点所在的那个包：认可秘密、算遮法、替换明文，全部只在这里。
const maskHome = "mask"

// maskMarkerFile 是本检查自己：它要认得那个记号才能找它，所以这一份文件不扫自己
// （只有这一个文件，不是整个检查包放行——按文件收窄，别处写星号照样报）。
const maskMarkerFile = "internal/guard/check_mask.go"

// checkMaskStar 是「遮」在源码里的记号：四处（改密的值、内联清单、采集输出里的明文、
// 清单副本）遮出来的样子必须是同一串，所以星号只许在 mask 包里成形。
const checkMaskStar = "****"

// CheckMask 是类七：遮的构造点只有一个。
//
// 判据（建站 07 规格第六节，开发者 2026-11-06：「遮蔽只有一种方法，而且只能从这个横切包
// 去遮蔽」）：除 `mask` 包与 `_test.go`，任何源码文件里出现遮的记号即算。
//
// 为什么按字面量判、不按调用关系判：这里要禁的**就是这个字面量**——别处写出它，说明那一处
// 自己造了一套遮法；调 `mask` 的人不会写星号，写星号的人一定绕开了 `mask`。判据按语法树取
// 字符串字面量，不按文本搜，注释与测试里的说明不误报。
func CheckMask(m *Module) []Violation {
	var out []Violation
	for _, p := range m.Packages {
		if p.Short == maskHome {
			continue
		}
		for _, f := range p.Files {
			if !scans(ClassMask, f) || f.Rel == maskMarkerFile {
				continue
			}
			ast.Inspect(f.AST, func(n ast.Node) bool {
				lit, ok := n.(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					return true
				}
				s, err := strconv.Unquote(lit.Value)
				if err != nil || !strings.Contains(s, checkMaskStar) {
					return true
				}
				out = append(out, f.violation(ClassMask, lit.Pos(),
					"自己拼了遮出来的样子（字符串里有 "+checkMaskStar+"）",
					"遮的构造点只有 mask 一个：别处写星号，同一个秘密在不同产物里就会是两个样子，改一处漏一处",
					"把要遮的东西交给 mask——遮一个值、遮一条内联记录、把明文从一段文本里擦掉，都是它的整件事"))
				return true
			})
		}
	}
	return out
}
