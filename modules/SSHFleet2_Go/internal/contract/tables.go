package contract

import "regexp"

// DangerRule 一条危险命令规则：名字、风险档、示例、说明，加编译好的正则。
//
// 正则由解析站编译（读表与校验是它的事），闸门只拿它匹配——同一份判据，两个来源的事不做。
type DangerRule struct {
	Name     string
	Level    string // 取值见 RiskForbidden／RiskHigh／RiskMedium／RiskLow
	Example  string
	Reason   string
	Enabled  bool
	Patterns []*regexp.Regexp
}

// Category 一条失败分类：分类名、关键词（顺序即匹配优先级）、可选的排查建议。
type Category struct {
	Name     string
	Keywords []string
	Tip      string
}

// PasswdStep 改密时的一条提示：远端问这些话时，喂哪个值、最多喂几次。
type PasswdStep struct {
	Keywords []string
	Value    string // 只认 current／new
	Max      int
}

// PasswdValueCurrent 与 PasswdValueNew 是改密提示词表里 value 的两个合法取值。
const (
	PasswdValueCurrent = "current"
	PasswdValueNew     = "new"
)

// Tables 是三份判据表加载之后的形态：解析站读一次、校验一次，随纸往下走。
type Tables struct {
	Danger           []DangerRule
	Categories       []Category // 第一组：目的命令没有退出码时用
	ExitCategories   []Category // 第二组：目的命令自己以非 0 收场时用
	PasswdSteps      []PasswdStep
	PasswdCategories []Category
}
