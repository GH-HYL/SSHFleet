package dangercheck

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/BurntSushi/toml"
)

// minRules 规则数量下限（防整份文件被误删/清空）。
const minRules = 10

// Rule 一条危险命令规则（字段与旧 YAML 一致，spec D11 语义不动）。
type Rule struct {
	Name      string `toml:"name"`
	Example   string `toml:"example"`
	Regex     string `toml:"regex"`
	RiskLevel string `toml:"risk_level"`
	Enabled   *bool  `toml:"enabled"`
}

func (r Rule) enabled() bool { return r.Enabled == nil || *r.Enabled }

// Rules 加载并编译后的规则集合。
type Rules struct {
	items    []Rule
	patterns []*regexp.Regexp
}

// Count 规则条数。
func (r *Rules) Count() int {
	if r == nil {
		return 0
	}
	return len(r.items)
}

type rulesFile struct {
	Rules []Rule `toml:"rules"`
}

// LoadRules 读取并校验规则文件：结构、条数下限、字段完整性、risk_level 取值、正则可编译。
// 校验口径与旧 check_dangerous_dict 一致（指名报错）。
func LoadRules(path string) (*Rules, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取危险命令检测规则失败：%s\n原因：%v", path, err)
	}
	var file rulesFile
	md, err := toml.Decode(string(data), &file)
	if err != nil {
		return nil, fmt.Errorf("危险命令检测规则解析失败：%s\n原因：%v", path, err)
	}
	if undecoded := md.Undecoded(); len(undecoded) > 0 {
		keys := make([]string, 0, len(undecoded))
		for _, k := range undecoded {
			keys = append(keys, k.String())
		}
		return nil, fmt.Errorf("危险命令检测规则含未识别字段：%s", strings.Join(keys, ", "))
	}
	return compileRules(file.Rules)
}

// compileRules 校验并编译规则列表（对位旧 check_dangerous_dict 的全部拦截项）。
func compileRules(rules []Rule) (*Rules, error) {
	if len(rules) == 0 {
		return nil, fmt.Errorf("危险命令检测规则为空，程序退出！")
	}
	if len(rules) < minRules {
		return nil, fmt.Errorf("危险命令检测规则不足（当前%d条，需要至少%d条），程序退出！", len(rules), minRules)
	}

	out := &Rules{}
	for idx, rule := range rules {
		label := rule.Name
		if label == "" {
			label = fmt.Sprintf("第%d条", idx+1)
		}
		for _, field := range []struct{ name, val string }{
			{"name", rule.Name},
			{"example", rule.Example},
			{"risk_level", rule.RiskLevel},
			{"regex", rule.Regex},
		} {
			if strings.TrimSpace(field.val) == "" {
				return nil, fmt.Errorf("rules %s 缺少字段或字段为空：'%s'，程序退出！", label, field.name)
			}
		}
		if _, ok := riskOrder[rule.RiskLevel]; !ok {
			return nil, fmt.Errorf("rules %s 的 risk_level 非法：'%s'（仅支持 forbidden/high/medium/low），程序退出！", label, rule.RiskLevel)
		}
		pattern, err := regexp.Compile("(?i)" + rule.Regex) // 对位旧 re.IGNORECASE
		if err != nil {
			return nil, fmt.Errorf("rules %s 的正则无法编译：%v，程序退出！", label, err)
		}
		out.items = append(out.items, rule)
		out.patterns = append(out.patterns, pattern)
	}
	return out, nil
}

// match 对归一化后的命令段做判定，返回命中的最高风险规则名与级别（无命中返回空串）。
func (r *Rules) match(normalized string) (string, string) {
	if r == nil || normalized == "" {
		return "", ""
	}
	bestName, bestLevel := "", ""
	for i, rule := range r.items {
		if !rule.enabled() {
			continue
		}
		if !r.patterns[i].MatchString(normalized) {
			continue
		}
		if bestLevel == "" || riskOrder[rule.RiskLevel] < riskOrder[bestLevel] {
			bestName, bestLevel = rule.Name, rule.RiskLevel
		}
	}
	return bestName, bestLevel
}
