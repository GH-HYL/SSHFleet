package parse

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"

	"sshfleet2/internal/contract"
	"sshfleet2/internal/message"
)

// 三份判据表的读取与校验：危险规则、错误分类、改密提示词。
//
// 读表归解析（「谁最懂那份材料谁来验」），闸门只拿编译好的判据去匹配——同一份事实不出现
// 在两个站里。判据表随第一张纸往下走（ADR-0035 决定五）。

const (
	dangerRulesPath    = "./config/keywords_dangerous.conf"
	errorKeywordsPath  = "./config/keywords_error.conf"
	passwdKeywordsPath = "./config/keywords_passwd.conf"
)

// ruleTextLimit 是规则里 example／reason 的显示宽度上限（一个汉字算两列）。
const ruleTextLimit = 30

// loadTables 读三份表。任何一份不成器都是「起不来」。
func loadTables(mode contract.Mode) (contract.Tables, *contract.CannotStart) {
	danger, fail := loadDangerRules()
	if fail != nil {
		return contract.Tables{}, fail
	}
	cats, exitCats, fail := loadErrorKeywords()
	if fail != nil {
		return contract.Tables{}, fail
	}
	steps, passwdCats, fail := loadPasswdKeywords()
	if fail != nil {
		return contract.Tables{}, fail
	}

	// 那条唯一的行为例外：改密模式下，「密码过期」这一分类没有关键词就是起不来，启动即报。
	// 老版跑到改密那一步才说；这一处是定过的行为变化（ADR-0034 决定七）。
	if mode == contract.ModePasswd && len(keywordsOf(cats, "密码过期")) == 0 && len(keywordsOf(exitCats, "密码过期")) == 0 {
		return contract.Tables{}, cannotStartReason("ssh",
			contract.CannotStartReason{Kind: contract.CannotStartPasswdNoSignal})
	}

	return contract.Tables{
		Danger:           danger,
		Categories:       cats,
		ExitCategories:   exitCats,
		PasswdSteps:      steps,
		PasswdCategories: passwdCats,
	}, nil
}

// keywordsOf 找一个分类的关键词。
func keywordsOf(cats []contract.Category, name string) []string {
	for _, c := range cats {
		if c.Name == name {
			return c.Keywords
		}
	}
	return nil
}

// dangerFail 排一屏「危险规则表不成器」：环节与外壳句照老版那一处，六条失败共用一个入口。
func dangerFail(reason contract.CannotStartReason) *contract.CannotStart {
	reason.Head = message.HeadDangerRules
	return cannotStartReason("dangercheck", reason)
}

// loadDangerRules 读危险规则表：六项字段、四档等级、正则编译、示例与说明的宽度。
func loadDangerRules() ([]contract.DangerRule, *contract.CannotStart) {
	data, err := os.ReadFile(dangerRulesPath)
	if err != nil {
		return nil, dangerFail(contract.CannotStartReason{
			Kind:   contract.CannotStartTableRead,
			Place:  "读取危险命令检测规则",
			Value:  dangerRulesPath,
			Origin: err.Error(),
		})
	}
	lines, err := tokenize(data)
	if err != nil {
		return nil, dangerFail(contract.CannotStartReason{
			Kind:   contract.CannotStartTableParse,
			Place:  "危险命令检测规则",
			Value:  dangerRulesPath,
			Origin: err.Error(),
		})
	}
	if !hasArrayTable(lines, "rules") {
		return nil, dangerFail(contract.CannotStartReason{
			Kind:   contract.CannotStartTableEmpty,
			Place:  "危险命令检测规则",
			Value:  "rules",
			Origin: dangerRulesPath,
		})
	}

	items := arrayItems(lines, "rules")
	out := make([]contract.DangerRule, 0, len(items))
	for i, item := range items {
		if unknown := unknownRuleKeys(item); len(unknown) > 0 {
			return nil, dangerFail(contract.CannotStartReason{
				Kind:  contract.CannotStartTableUnknownField,
				Place: "危险命令检测规则",
				Value: strings.Join(unknown, ", "),
				Origin: "规则只认 name / example / reason / regex / risk_level / enabled 六项，" +
					"把它删掉或改成这六项之一",
			})
		}
		rule, bad := buildRule(item, i+1)
		if bad != nil {
			return nil, dangerFail(*bad)
		}
		out = append(out, rule)
	}
	return out, nil
}

// unknownRuleKeys 规则条目里不认识的键。
func unknownRuleKeys(item map[string]string) []string {
	known := map[string]bool{"name": true, "example": true, "reason": true, "regex": true, "risk_level": true, "enabled": true}
	var out []string
	for k := range item {
		if !known[k] {
			out = append(out, k)
		}
	}
	sortStrings(out)
	return out
}

// buildRule 校验并编译一条规则：哪一格对不上就交回「哪一类 ＋ 几个参数」，句子由 message 拼。
func buildRule(item map[string]string, index int) (contract.DangerRule, *contract.CannotStartReason) {
	rule := contract.DangerRule{
		Name:    unquote(item["name"]),
		Example: unquote(item["example"]),
		Reason:  unquote(item["reason"]),
		Level:   unquote(item["risk_level"]),
		Enabled: true,
	}
	if strings.TrimSpace(rule.Name) == "" {
		return rule, &contract.CannotStartReason{Kind: contract.CannotStartDangerNoName, Index: index}
	}
	if !validRiskLevel(rule.Level) {
		return rule, &contract.CannotStartReason{
			Kind:  contract.CannotStartDangerBadLevel,
			Index: index,
			Value: rule.Level,
		}
	}
	if w := message.DisplayWidth(rule.Example); w > ruleTextLimit {
		return rule, &contract.CannotStartReason{
			Kind:   contract.CannotStartDangerExampleTooWide,
			Index:  index,
			Value:  rule.Name,
			Limit:  ruleTextLimit,
			Origin: strconv.Itoa(w),
		}
	}
	if w := message.DisplayWidth(rule.Reason); w > ruleTextLimit {
		return rule, &contract.CannotStartReason{
			Kind:   contract.CannotStartDangerReasonTooWide,
			Index:  index,
			Value:  rule.Name,
			Limit:  ruleTextLimit,
			Origin: strconv.Itoa(w),
		}
	}
	if v, ok := item["enabled"]; ok {
		switch strings.TrimSpace(unquote(v)) {
		case "false":
			rule.Enabled = false
		case "true":
			rule.Enabled = true
		default:
			return rule, &contract.CannotStartReason{
				Kind:  contract.CannotStartDangerBadEnabled,
				Index: index,
				Value: rule.Name,
			}
		}
	}
	raw := unquote(item["regex"])
	if strings.TrimSpace(raw) == "" {
		return rule, &contract.CannotStartReason{
			Kind:  contract.CannotStartDangerNoRegex,
			Index: index,
			Value: rule.Name,
		}
	}
	patterns := splitArray(raw)
	if len(patterns) == 0 {
		return rule, &contract.CannotStartReason{
			Kind:  contract.CannotStartDangerEmptyRegex,
			Index: index,
			Value: rule.Name,
		}
	}
	for _, p := range patterns {
		// 大小写不敏感：老版编译时统一加 `(?i)`（对位旧实现的 re.IGNORECASE），
		// 不照搬就会漏掉 `RM -RF /etc` 这一类写法。
		re, err := regexp.Compile("(?i)" + p)
		if err != nil {
			return rule, &contract.CannotStartReason{
				Kind:   contract.CannotStartDangerBadRegex,
				Index:  index,
				Value:  rule.Name,
				Origin: err.Error(),
			}
		}
		rule.Patterns = append(rule.Patterns, re)
	}
	return rule, nil
}

// validRiskLevel 判等级在不在四档里——取值来自契约常量，别处不再写第二份。
func validRiskLevel(level string) bool {
	switch level {
	case contract.RiskForbidden, contract.RiskHigh, contract.RiskMedium, contract.RiskLow:
		return true
	}
	return false
}

// 判据文件里两个数组标题的键名。单数，与 `keywords_passwd.conf` 的 `[[category]]` 同一套
// 写法：一份表一个标题，名字对得上眼就少一次「这个 s 是哪个」的迟疑。
const (
	errorCategoriesKey = "category"
	exitCategoriesKey  = "exit_code_category"
)

// loadErrorKeywords 读错误分类表：两块，第一组不可为空。
func loadErrorKeywords() ([]contract.Category, []contract.Category, *contract.CannotStart) {
	data, err := os.ReadFile(errorKeywordsPath)
	if err != nil {
		return nil, nil, cannotStartReason("verdict", contract.CannotStartReason{
			Kind:   contract.CannotStartTableRead,
			Place:  "读取错误分类关键词",
			Value:  errorKeywordsPath,
			Origin: err.Error(),
		})
	}
	lines, err := tokenize(data)
	if err != nil {
		return nil, nil, cannotStartReason("verdict", contract.CannotStartReason{
			Kind:   contract.CannotStartTableParse,
			Place:  "错误分类关键词",
			Value:  errorKeywordsPath,
			Origin: err.Error(),
		})
	}
	for _, section := range []string{errorCategoriesKey, exitCategoriesKey} {
		if unknown := unknownCategoryKeys(lines, section); len(unknown) > 0 {
			return nil, nil, cannotStartReason("verdict", contract.CannotStartReason{
				Kind:  contract.CannotStartTableUnknownField,
				Place: "错误分类关键词",
				Value: strings.Join(unknown, ", "),
			})
		}
	}

	cats, bad := buildCategories(lines, errorCategoriesKey)
	if bad != nil {
		return nil, nil, cannotStartReason("verdict", *bad)
	}
	if len(cats) == 0 {
		return nil, nil, cannotStartReason("verdict", contract.CannotStartReason{
			Kind:  contract.CannotStartCategoriesEmpty,
			Place: errorKeywordsPath,
		})
	}
	exitCats, bad := buildCategories(lines, exitCategoriesKey)
	if bad != nil {
		return nil, nil, cannotStartReason("verdict", *bad)
	}
	return cats, exitCats, nil
}

// unknownCategoryKeys 分类条目里不认识的键。
func unknownCategoryKeys(lines []tomlLine, section string) []string {
	var out []string
	for _, item := range arrayItems(lines, section) {
		for k := range item {
			if k != "name" && k != "keywords" && k != "tip" {
				out = append(out, k)
			}
		}
	}
	sortStrings(out)
	return out
}

// buildCategories 把某个数组表建成分类清单：名字非空、关键词里不许有空项。
func buildCategories(lines []tomlLine, section string) ([]contract.Category, *contract.CannotStartReason) {
	var out []contract.Category
	for i, item := range arrayItems(lines, section) {
		c := contract.Category{
			Name:     unquote(item["name"]),
			Keywords: splitArray(item["keywords"]),
			Tip:      unquote(item["tip"]),
		}
		if strings.TrimSpace(c.Name) == "" {
			return nil, &contract.CannotStartReason{
				Kind:  contract.CannotStartKeywordCatNoName,
				Index: i + 1,
			}
		}
		for _, kw := range c.Keywords {
			if strings.TrimSpace(kw) == "" {
				return nil, &contract.CannotStartReason{
					Kind:  contract.CannotStartKeywordCatEmptyItem,
					Value: c.Name,
				}
			}
		}
		out = append(out, c)
	}
	return out, nil
}

// loadPasswdKeywords 读改密提示词表：step 与 category 两块。
func loadPasswdKeywords() ([]contract.PasswdStep, []contract.Category, *contract.CannotStart) {
	data, err := os.ReadFile(passwdKeywordsPath)
	if err != nil {
		return nil, nil, cannotStartReason("config", contract.CannotStartReason{
			Kind:   contract.CannotStartTableRead,
			Place:  "读取改密配置",
			Value:  passwdKeywordsPath,
			Origin: err.Error(),
		})
	}
	lines, err := tokenize(data)
	if err != nil {
		return nil, nil, cannotStartReason("config", contract.CannotStartReason{
			Kind:   contract.CannotStartTableParse,
			Place:  "改密配置",
			Value:  passwdKeywordsPath,
			Origin: err.Error(),
		})
	}
	for _, section := range []string{"step", "category"} {
		for _, item := range arrayItems(lines, section) {
			for k := range item {
				if section == "step" && (k == "keywords" || k == "value" || k == "max") {
					continue
				}
				if section == "category" && (k == "name" || k == "keywords" || k == "tip") {
					continue
				}
				return nil, nil, cannotStartReason("config", contract.CannotStartReason{
					Kind:  contract.CannotStartTableUnknownField,
					Place: "改密配置",
					Value: k,
				})
			}
		}
	}

	stepItems := arrayItems(lines, "step")
	if len(stepItems) == 0 {
		return nil, nil, cannotStartReason("config", contract.CannotStartReason{
			Kind:   contract.CannotStartPasswdEmptyStep,
			Origin: passwdKeywordsPath,
		})
	}
	var steps []contract.PasswdStep
	for i, item := range stepItems {
		step := contract.PasswdStep{
			Keywords: splitArray(item["keywords"]),
			Value:    unquote(item["value"]),
		}
		if len(step.Keywords) == 0 {
			return nil, nil, cannotStartReason("config", contract.CannotStartReason{
				Kind:  contract.CannotStartPasswdStepNoKeywords,
				Place: "step",
				Index: i + 1,
			})
		}
		for _, kw := range step.Keywords {
			if strings.TrimSpace(kw) == "" {
				return nil, nil, cannotStartReason("config", contract.CannotStartReason{
					Kind:  contract.CannotStartPasswdStepEmptyItem,
					Place: "step",
					Index: i + 1,
				})
			}
		}
		switch step.Value {
		case contract.PasswdValueCurrent, contract.PasswdValueNew:
		default:
			return nil, nil, cannotStartReason("config", contract.CannotStartReason{
				Kind:  contract.CannotStartPasswdStepBadValue,
				Place: "step",
				Index: i + 1,
				Value: step.Value,
			})
		}
		if maxRaw := strings.TrimSpace(unquote(item["max"])); maxRaw != "" {
			n, err := atoi(maxRaw)
			if err != nil || n < 1 {
				return nil, nil, cannotStartReason("config", contract.CannotStartReason{
					Kind:  contract.CannotStartPasswdStepBadMax,
					Place: "step",
					Index: i + 1,
					Value: maxRaw,
				})
			}
			step.Max = n
		}
		if step.Max < 1 {
			return nil, nil, cannotStartReason("config", contract.CannotStartReason{
				Kind:  contract.CannotStartPasswdStepBadMax,
				Place: "step",
				Index: i + 1,
				Value: "0",
			})
		}
		steps = append(steps, step)
	}

	var cats []contract.Category
	for i, item := range arrayItems(lines, "category") {
		c := contract.Category{
			Name:     unquote(item["name"]),
			Keywords: splitArray(item["keywords"]),
			Tip:      unquote(item["tip"]),
		}
		if strings.TrimSpace(c.Name) == "" {
			return nil, nil, cannotStartReason("config", contract.CannotStartReason{
				Kind:  contract.CannotStartPasswdCatNoName,
				Place: "category",
				Index: i + 1,
			})
		}
		for _, kw := range c.Keywords {
			if strings.TrimSpace(kw) == "" {
				return nil, nil, cannotStartReason("config", contract.CannotStartReason{
					Kind:  contract.CannotStartPasswdCatEmptyItem,
					Place: "category",
					Index: i + 1,
					Value: c.Name,
				})
			}
		}
		cats = append(cats, c)
	}
	return steps, cats, nil
}

// atoi 把一段数字文本转成整数。
func atoi(s string) (int, error) {
	n := 0
	if s == "" {
		return 0, fmt.Errorf("空值")
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0, fmt.Errorf("不是整数")
		}
		n = n*10 + int(r-'0')
	}
	return n, nil
}
