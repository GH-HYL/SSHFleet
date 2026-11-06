package guard

// 两个失败类型的家。名字只在这里写一次：契约包那行问规则表也能问到，但闸门是五个站之一，
// 规则表的「类」分不出哪一个是它，只能点名。
const (
	cannotStartHome = "contract" // 「起不来」住契约包（ADR-0034 决定三）
	refusalHome     = "gate"     // 「拒绝」只住闸门
)

// DefaultFailures 是第五类要判的两处（ADR-0034 决定三）：
// 起不来谁都能造、闸门里不许造；拒绝只许闸门造。
func DefaultFailures() []FailureSite {
	return []FailureSite{
		{Pkg: cannotStartHome, Type: "CannotStart", NeverIn: []string{refusalHome}},
		{Pkg: refusalHome, Type: "Refusal", OnlyIn: []string{refusalHome}},
	}
}

// Check 读一个模块，把六类跑一遍，返回没被台账放行的违规。
func Check(t Target) ([]Violation, error) {
	m, err := Load(t)
	if err != nil {
		return nil, err
	}
	return CheckAll(m), nil
}

// CheckAll 在已读进来的模块上按类号顺序跑全。
func CheckAll(m *Module) []Violation {
	var vs []Violation
	vs = append(vs, CheckTerminal(m)...) // 类一
	vs = append(vs, CheckDeps(m)...)     // 类二
	vs = append(vs, CheckExit(m)...)     // 类三
	vs = append(vs, CheckContract(m)...) // 类四
	vs = append(vs, CheckFailure(m)...)  // 类五
	vs = append(vs, CheckTestSite(m)...) // 类六之一
	vs = append(vs, CheckHandoff(m)...)  // 类六之二
	vs = append(vs, CheckMask(m)...)     // 类七
	vs = append(vs, CheckRedline(m)...)  // 红线名单与指纹

	kept, stale := Apply(vs, m.Ledger)
	for _, e := range stale {
		kept = append(kept, violationAt(e.Rule, e.Where,
			"台账这一行已经盖不住任何违规了",
			"放行是让一条已知的越界暂时过去；那一处改好之后台账还留着，条目就只会越攒越多，翻的人也就不翻了",
			"把这一行删掉——删它同样要开发者点头"))
	}
	return kept
}
