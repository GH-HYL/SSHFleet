package guard

import (
	"reflect"
	"strings"
	"testing"

	"sshfleet2/internal/contract"
)

// 红线：第二张纸里没有「成败」这个格子——成败只由结算站按「分类是不是本模式的成功分类」
// 解释（ADR-0035 决定一、四）。
//
// 只判形状：字段名里带 Success／Failed／Verdict 就算红。值对不对、算得准不准，一条都不在
// 这里判——那是自定义区的事。这条管的是「没人偷偷给执行站加一个结论位」：有了它，结算站
// 就不再是唯一解释成败的地方，两个来源迟早对不上。
func TestRedlineResultHasNoVerdict(t *testing.T) {
	banned := []string{"Success", "Failed", "Verdict"}
	for _, paper := range []any{contract.NodeResult{}, contract.ResultPaper{}} {
		typ := reflect.TypeOf(paper)
		for i := 0; i < typ.NumField(); i++ {
			name := typ.Field(i).Name
			for _, word := range banned {
				if strings.Contains(name, word) {
					t.Fatalf("%s 里出现了「成败」字段 %s：成败只由结算站解释", typ.Name(), name)
				}
			}
		}
	}
}
