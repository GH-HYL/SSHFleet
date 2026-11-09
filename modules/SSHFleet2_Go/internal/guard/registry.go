package guard

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// EntryFiles 是闸门的入口登记——自反登记的第一样：检查入口文件本身。
//
// 为什么要有这张表：几十条红线全住在测试里，而测试靠入口跑起来。入口文件一被删，
// 「跑测试就红」就静默变成「跑测试全绿」。登记把入口本身也变成被测对象：少一个文件，
// 剩下的入口照样会跑 CheckAll，于是这一条红。
//
// 第二、三样（红线名单、每条红线的测试名）不在这里再抄一遍：名单是 redline 表本身，
// 测试名由 CheckRedline 拿代码去对，两边都不另存一份，免得这张表跟真名单各说各话。
//
// 路径写模块内相对路径，与台账「位置」同形。
var EntryFiles = []string{
	"internal/guard/project_test.go",  // 真工程整跑：CheckAll 的触发点
	"internal/guard/registry_test.go", // 登记自己的触发点
	ledgerPath,                        // 放行台账：读不到就等于检查没跑
}

// CheckRegistry 判登记的入口文件是不是都还在。
//
// 它不读语法树：被删掉的文件解析不出任何节点，只能从「该在的不在」来认。
// 只对自己这个模块生效——样本模块（testdata）里没有检查包，登记在那儿没有意义。
func CheckRegistry(m *Module) []Violation {
	if m.PkgOfKind(KindCheck) == nil {
		return nil
	}
	var out []Violation
	for _, rel := range EntryFiles {
		if _, err := os.Stat(filepath.Join(m.Dir, filepath.FromSlash(rel))); err == nil {
			continue
		}
		out = append(out, violationAt(ClassRedline, rel,
			fmt.Sprintf("登记缺一项：入口（%s 不在了）", filepath.Base(rel)),
			"闸门靠这个文件跑起来：它一被删，整道检查就静默停下，而 go test 仍是全绿——"+
				"「跑测试就红」这句话正是这样被拆掉的",
			"把文件补回来；确要改这份登记，走议题加 ADR"))
	}
	return out
}

// CheckLedger 判台账每一行是不是四格都填了：规则、位置、为什么放、谁批的。
//
// 四格空一格，放行就成了一张没署名的条子：事后没人说得清为什么要放、谁点的头。
// 报「第几条」而不报行号——台账的行号随表头增删漂移，条目序号不漂。
func CheckLedger(ledger []Exception) []Violation {
	var out []Violation
	for i, e := range ledger {
		var missing []string
		for _, c := range []struct {
			name string
			got  string
		}{
			{ledgerCols[0], e.Rule},
			{ledgerCols[1], e.Where},
			{ledgerCols[2], e.Why},
			{ledgerCols[3], e.Who},
		} {
			if strings.TrimSpace(c.got) == "" {
				missing = append(missing, c.name)
			}
		}
		if len(missing) == 0 {
			continue
		}
		out = append(out, violationAt(ClassRedline, ledgerPath,
			fmt.Sprintf("台账第 %d 条缺了「%s」", i+1, strings.Join(missing, "」「")),
			"放行是让人给越界签字：格不全的放行等于一张没写理由、没署名的条子，事后无从追",
			"补全这几格；补不了就把这一行删掉——删它同样要开发者点头"))
	}
	return out
}
