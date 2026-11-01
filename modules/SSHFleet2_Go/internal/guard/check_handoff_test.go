package guard

import "testing"

// 类六之二：闸门那张纸比解析那张多一格数据要报；只多一格结论不报。
func TestCheckHandoff(t *testing.T) {
	m := loadFixture(t, "handoff")
	m.Pairs = []PaperPair{
		{FromPkg: "contract", FromType: "ParsePlan", ToPkg: "contract", ToType: "GatePlan"},
		{FromPkg: "contract", FromType: "PlanX", ToPkg: "contract", ToType: "PlanY"},
	}
	vs := CheckHandoff(m)
	if len(vs) != 1 {
		t.Fatalf("want 1 条（GatePlan 多了 Extra），got %d：\n%s", len(vs), Report(vs))
	}
	if !anyContains(vs, "Extra") {
		t.Errorf("没点出多出来的那一格：\n%s", Report(vs))
	}
}

// 对账表里写着一对纸、代码里没有，也要报。
func TestCheckHandoffMissingPaper(t *testing.T) {
	m := loadFixture(t, "handoff")
	m.Pairs = []PaperPair{{FromPkg: "contract", FromType: "NoSuchPaper", ToPkg: "contract", ToType: "PlanX"}}
	vs := CheckHandoff(m)
	if len(vs) != 1 || !anyContains(vs, "不在了") {
		t.Fatalf("want 一条「不在了」，got：%v", vs)
	}
}
