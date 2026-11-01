package contract

// ParsePlan 是解析交闸门那张纸。
type ParsePlan struct {
	IP  string
	Cmd string
}

// GatePlan 比它多一格 Extra——类六之二的坏样本。
type GatePlan struct {
	IP         string
	Cmd        string
	Extra      string
	Conclusion string
}

// PlanX 与 PlanY 是一对规矩的纸。
type PlanX struct {
	IP string
}

// PlanY 只多了结论那一格。
type PlanY struct {
	IP         string
	Conclusion string
}
