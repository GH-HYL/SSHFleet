package contract

// DangerMatch 一处命中：同一条规则的全部命中合成一条（行号升序去重，内容取第一处）。
type DangerMatch struct {
	Lines    []int
	Content  string
	RuleName string
	Level    string
	Example  string
	Reason   string
}

// DangerReport 一次危险检测的结果。命中的先后由闸门排好，这里只装着。
type DangerReport struct {
	Matches    []DangerMatch
	IsScript   bool
	ScriptPath string
}

// HasForbidden 报「最高那一档是不是禁止」。契约不许有函数，所以这里是字段——由闸门判定时写好。
type DangerVerdict struct {
	Report    DangerReport
	Forbidden bool
	Highest   string
}
