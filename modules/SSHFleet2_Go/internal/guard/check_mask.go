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

// mate 判一段文本里有没有遮的记号。
//
// 记号长什么样只由 `mask` 一处写死（一个字面量）；本检查不抄那个字面量——抄一份就多一个
// 出处，`mask` 改了记号这边还照旧扫，类七会静默失效。所以用构造式拼出「四个连续星号」，
// 语法树里没有字面量，类七也扫不到自己。
func mate(s string) bool { return strings.Contains(s, strings.Repeat("*", 4)) }

// CheckMask 是类七：遮的构造点只有一个。
//
// 判据（建站 07 规格第六节，开发者 2026-11-06：「遮蔽只有一种方法，而且只能从这个横切包
// 去遮蔽」）：除 `mask` 包与 `_test.go`，任何源码文件里出现遮的记号即算。
//
// 为什么按字面量判、不按调用关系判：这里要禁的**就是这个记号**——别处写出它，说明那一处
// 自己造了一套遮法；调 `mask` 的人不会写星号，写星号的人一定绕开了 `mask`。判据按语法树取
// 字符串字面量，不按文本搜，注释与测试里的说明不误报。
//
// 另带一条自检：`mask` 包里必须还找得到那个记号。找不到说明它换了写法，而本检查还在按旧
// 形态扫——那时类七一条都报不出来，比漏报更糟。
func CheckMask(m *Module) []Violation {
	var out []Violation
	homeSeen, homeFound := false, false
	for _, p := range m.Packages {
		if p.Short == maskHome {
			homeSeen = true
			if maskHasMarker(p) {
				homeFound = true
			}
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
				if err != nil || !mate(s) {
					return true
				}
				out = append(out, f.violation(ClassMask, lit.Pos(),
					"自己拼了遮出来的样子（字符串里有四个连续星号）",
					"遮的构造点只有 mask 一个：别处写星号，同一个秘密在不同产物里就会是两个样子，改一处漏一处",
					"把要遮的东西交给 mask——遮一个值、遮一条内联记录、把明文从一段文本里擦掉，都是它的整件事"))
				return true
			})
		}
	}
	// 自检只在模块里真有这个包时才做：样本模块里没有 mask，缺包归包清单那条检查报。
	if homeSeen && !homeFound {
		out = append(out, violationAt(ClassMask, "internal/"+maskHome,
			"mask 包里已经找不到遮的记号了",
			"本检查按「四个连续星号」认那个记号；mask 换了写法，类七就一条都报不出来，等于这条约束失效",
			"把 check_mask.go 里的 mate 改成跟着新写法认"))
	}
	return out
}

// maskHasMarker 看 mask 包里还有没有那个记号的字面量。
func maskHasMarker(p *Package) bool {
	for _, f := range p.Files {
		found := false
		ast.Inspect(f.AST, func(n ast.Node) bool {
			lit, ok := n.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			if s, err := strconv.Unquote(lit.Value); err == nil && mate(s) {
				found = true
				return false
			}
			return true
		})
		if found {
			return true
		}
	}
	return false
}
