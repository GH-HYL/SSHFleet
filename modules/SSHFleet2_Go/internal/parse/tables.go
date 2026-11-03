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

// unknownCategoryField 是错误分类表两条「含未识别字段」共用的句子头。
const unknownCategoryField = "错误分类关键词含未识别字段："

// maxMustBePositive 是改密 max 那一格的句子：算出来的值与非正数都走它。
// 取值那半句向 message 的构造点要，不在这里抄一份。
func maxMustBePositive(raw string) string {
	return message.CurrentValue("max 必须是正整数", raw)
}

// passwdIssue 给「改密提示词表第 N 条」的失败排一句：前缀只在这里拼，七条失败共用。
func passwdIssue(kind string, i int, format string, args ...any) string {
	return "改密配置第 " + strconv.Itoa(i) + " 条 " + kind + "：" + fmt.Sprintf(format, args...)
}

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
		body := message.Problem(
			"改密缺少入场信号",
			"判据文件里「密码过期」分类没有关键词，工具认不出哪台机器要改密",
			"该分类在 config/keywords_error.conf，别删它")
		return contract.Tables{}, cannotStart("ssh", "", body)
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

// dangerFail 排一屏「危险规则表不成器」：环节与外壳句照老版那一处，五条失败共用一个入口。
func dangerFail(body string) *contract.CannotStart {
	return cannotStart("dangercheck", message.HeadDangerRules, body)
}

// loadDangerRules 读危险规则表：六项字段、四档等级、正则编译、示例与说明的宽度。
func loadDangerRules() ([]contract.DangerRule, *contract.CannotStart) {
	data, err := os.ReadFile(dangerRulesPath)
	if err != nil {
		return nil, dangerFail(
			message.ProblemReason("读取危险命令检测规则失败："+dangerRulesPath, err.Error()))
	}
	lines, err := tokenize(data)
	if err != nil {
		return nil, dangerFail(
			message.ProblemReason("危险命令检测规则解析失败："+dangerRulesPath, err.Error()))
	}
	if !hasArrayTable(lines, "rules") {
		return nil, dangerFail(
			fmt.Sprintf("危险命令检测规则里一条 rules 都没有：%s", dangerRulesPath))
	}

	items := arrayItems(lines, "rules")
	out := make([]contract.DangerRule, 0, len(items))
	for i, item := range items {
		if unknown := unknownRuleKeys(item); len(unknown) > 0 {
			return nil, dangerFail(
				message.ProblemOrigin(
					fmt.Sprintf("危险命令检测规则含未识别字段：%s", strings.Join(unknown, ", ")),
					"规则只认 name / example / reason / regex / risk_level / enabled 六项，把它删掉或改成这六项之一"))
		}
		rule, err := buildRule(item)
		if err != nil {
			return nil, dangerFail(
				fmt.Sprintf("危险命令检测规则第 %d 条：%v", i+1, err))
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

// buildRule 校验并编译一条规则。
func buildRule(item map[string]string) (contract.DangerRule, error) {
	rule := contract.DangerRule{
		Name:    unquote(item["name"]),
		Example: unquote(item["example"]),
		Reason:  unquote(item["reason"]),
		Level:   unquote(item["risk_level"]),
		Enabled: true,
	}
	if strings.TrimSpace(rule.Name) == "" {
		return rule, fmt.Errorf("缺少 name")
	}
	if !validRiskLevel(rule.Level) {
		return rule, fmt.Errorf("%s", message.CurrentValue(
			fmt.Sprintf("risk_level 只能写 %s / %s / %s / %s",
				contract.RiskForbidden, contract.RiskHigh, contract.RiskMedium, contract.RiskLow),
			rule.Level))
	}
	if w := message.DisplayWidth(rule.Example); w > ruleTextLimit {
		return rule, fmt.Errorf("%s 的 example 有 %d 列宽，超过上限 %d 列（一个汉字算两列）", rule.Name, w, ruleTextLimit)
	}
	if w := message.DisplayWidth(rule.Reason); w > ruleTextLimit {
		return rule, fmt.Errorf("%s 的 reason 有 %d 列宽，超过上限 %d 列（一个汉字算两列）", rule.Name, w, ruleTextLimit)
	}
	if v, ok := item["enabled"]; ok {
		switch strings.TrimSpace(unquote(v)) {
		case "false":
			rule.Enabled = false
		case "true":
			rule.Enabled = true
		default:
			return rule, fmt.Errorf("%s 的 enabled 只认 true / false", rule.Name)
		}
	}
	raw := unquote(item["regex"])
	if strings.TrimSpace(raw) == "" {
		return rule, fmt.Errorf("%s 没有 regex", rule.Name)
	}
	patterns := splitArray(raw)
	if len(patterns) == 0 {
		return rule, fmt.Errorf("%s 的 regex 是空的", rule.Name)
	}
	for _, p := range patterns {
		// 大小写不敏感：老版编译时统一加 `(?i)`（对位旧实现的 re.IGNORECASE），
		// 不照搬就会漏掉 `RM -RF /etc` 这一类写法。
		re, err := regexp.Compile("(?i)" + p)
		if err != nil {
			return rule, fmt.Errorf("%s 的一条正则编译不过：%v", rule.Name, err)
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

// loadErrorKeywords 读错误分类表：两块，第一组不可为空。
func loadErrorKeywords() ([]contract.Category, []contract.Category, *contract.CannotStart) {
	data, err := os.ReadFile(errorKeywordsPath)
	if err != nil {
		return nil, nil, cannotStart("verdict", "",
			message.ProblemReason("读取错误分类关键词失败："+errorKeywordsPath, err.Error()))
	}
	lines, err := tokenize(data)
	if err != nil {
		return nil, nil, cannotStart("verdict", "",
			message.ProblemReason("错误分类关键词解析失败："+errorKeywordsPath, err.Error()))
	}
	if unknown := unknownCategoryKeys(lines, "categories"); len(unknown) > 0 {
		return nil, nil, cannotStart("verdict", "", unknownCategoryField+strings.Join(unknown, ", "))
	}
	if unknown := unknownCategoryKeys(lines, "exit_code_categories"); len(unknown) > 0 {
		return nil, nil, cannotStart("verdict", "", unknownCategoryField+strings.Join(unknown, ", "))
	}

	cats, err := buildCategories(lines, "categories")
	if err != nil {
		return nil, nil, cannotStart("verdict", "", err.Error())
	}
	if len(cats) == 0 {
		return nil, nil, cannotStart("verdict", "", "错误分类关键词为空："+errorKeywordsPath)
	}
	exitCats, err := buildCategories(lines, "exit_code_categories")
	if err != nil {
		return nil, nil, cannotStart("verdict", "", err.Error())
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
func buildCategories(lines []tomlLine, section string) ([]contract.Category, error) {
	var out []contract.Category
	for i, item := range arrayItems(lines, section) {
		c := contract.Category{
			Name:     unquote(item["name"]),
			Keywords: splitArray(item["keywords"]),
			Tip:      unquote(item["tip"]),
		}
		if strings.TrimSpace(c.Name) == "" {
			return nil, fmt.Errorf("错误分类关键词第 %d 条缺少 name", i+1)
		}
		for _, kw := range c.Keywords {
			if strings.TrimSpace(kw) == "" {
				return nil, fmt.Errorf("错误分类关键词里 %s 的 keywords 有空项", c.Name)
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
		return nil, nil, cannotStart("config", "",
			message.ProblemReason("读取改密配置失败："+passwdKeywordsPath, err.Error()))
	}
	lines, err := tokenize(data)
	if err != nil {
		return nil, nil, cannotStart("config", "",
			message.ProblemReason("改密配置解析失败："+passwdKeywordsPath, err.Error()))
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
				return nil, nil, cannotStart("config", "", "改密配置含未识别字段："+k)
			}
		}
	}

	stepItems := arrayItems(lines, "step")
	if len(stepItems) == 0 {
		return nil, nil, cannotStart("config", "",
			fmt.Sprintf("%s", message.ProblemHint(
				"改密配置里一条 step 都没有："+passwdKeywordsPath,
				"至少留一条 [[step]]")))
	}
	var steps []contract.PasswdStep
	for i, item := range stepItems {
		step := contract.PasswdStep{
			Keywords: splitArray(item["keywords"]),
			Value:    unquote(item["value"]),
		}
		if len(step.Keywords) == 0 {
			return nil, nil, cannotStart("config", "", passwdIssue("step", i+1, "缺少 keywords（提示原文）"))
		}
		for _, kw := range step.Keywords {
			if strings.TrimSpace(kw) == "" {
				return nil, nil, cannotStart("config", "", passwdIssue("step", i+1, "keywords 里有空项"))
			}
		}
		switch step.Value {
		case contract.PasswdValueCurrent, contract.PasswdValueNew:
		default:
			return nil, nil, cannotStart("config", "", passwdIssue("step", i+1,
				"%s",
				message.CurrentValue(fmt.Sprintf("value 只能写 %s 或 %s",
					contract.PasswdValueCurrent, contract.PasswdValueNew), step.Value)))
		}
		if maxRaw := strings.TrimSpace(unquote(item["max"])); maxRaw != "" {
			n, err := atoi(maxRaw)
			if err != nil || n < 1 {
				return nil, nil, cannotStart("config", "", passwdIssue("step", i+1, "%s", maxMustBePositive(maxRaw)))
			}
			step.Max = n
		}
		if step.Max < 1 {
			return nil, nil, cannotStart("config", "", passwdIssue("step", i+1, "%s", maxMustBePositive("0")))
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
			return nil, nil, cannotStart("config", "", passwdIssue("category", i+1, "缺少 name（分类名）"))
		}
		for _, kw := range c.Keywords {
			if strings.TrimSpace(kw) == "" {
				return nil, nil, cannotStart("config", "", passwdIssue("category", i+1, "%s 的 keywords 里有空项", c.Name))
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
