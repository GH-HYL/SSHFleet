package dangercheck

import (
	"errors"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"

	"sshfleet/internal/common"
)

// 规则说明类字段（example / reason）的显示列数上限：一个汉字算两列。
// 超了就加载失败并指名是哪条——截断是不声不响的，使用者看到半句话会以为是程序算错了。
const ruleTextLimit = 30

// Rule 一条危险命令规则。
//
// Regex 是**字符串数组**：一条规则可以挂多条正则，命中任意一条即该规则命中（定案 A）。
// 只挂一条也写成数组——单条只是特例，不留「字符串也能写」的第二条路。
type Rule struct {
	Name      string   `toml:"name"`
	Example   string   `toml:"example"`
	Reason    string   `toml:"reason"`
	Regex     []string `toml:"regex"`
	RiskLevel string   `toml:"risk_level"`
	Enabled   *bool    `toml:"enabled"`
}

func (r Rule) enabled() bool { return r.Enabled == nil || *r.Enabled }

// Rules 加载并编译后的规则集合。items 与 compiled 一一对应。
type Rules struct {
	items    []Rule
	compiled [][]*regexp.Regexp
}

type rulesFile struct {
	Rules []Rule `toml:"rules"`
}

// LoadRules 读取并校验规则文件（定案 O：每条报错都能定位、能理解、能处置）。
func LoadRules(path string) (*Rules, error) {
	file, raw, err := readRulesFile(path)
	if err != nil {
		return nil, err
	}
	return compileRulesWithRaw(file.Rules, raw)
}

// readRulesFile 读文件并解出规则条目；raw 保留每条规则的原始字段，用于给出准确的报错。
func readRulesFile(path string) (rulesFile, []map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return rulesFile{}, nil, common.ErrUserReason(fmt.Sprintf("读取危险命令检测规则失败：%s", path), err.Error())
	}
	var file rulesFile
	md, err := toml.Decode(string(data), &file)
	if err != nil {
		return rulesFile{}, nil, fmt.Errorf("%s\n%s",
			common.ErrUserReason(fmt.Sprintf("危险命令检测规则解析失败：%s", path), err.Error()),
			decodeHint(err))
	}
	if undecoded := md.Undecoded(); len(undecoded) > 0 {
		keys := make([]string, 0, len(undecoded))
		for _, k := range undecoded {
			keys = append(keys, k.String())
		}
		return rulesFile{}, nil, fmt.Errorf(
			"危险命令检测规则含未识别字段：%s\n出处：规则只认 name / example / reason / regex / risk_level / enabled 六项，把它删掉或改成这六项之一",
			strings.Join(keys, ", "))
	}
	var raw struct {
		Rules []map[string]any `toml:"rules"`
	}
	if _, err := toml.Decode(string(data), &raw); err != nil {
		return rulesFile{}, nil, common.ErrUserReason(fmt.Sprintf("危险命令检测规则解析失败：%s", path), err.Error())
	}
	return file, raw.Rules, nil
}

// compileRules 校验并编译规则列表（只拿解码后的结构，报错文案不含「原写法」这类细节）。
func compileRules(rules []Rule) (*Rules, error) {
	return compileRulesWithRaw(rules, nil)
}

// compileRulesWithRaw 是 compileRules 的完整版：raw 与 rules 一一对应，用来分辨某个字段
// 到底是「没写」还是「写成了别的类型」（例如 regex 写成了字符串）。
func compileRulesWithRaw(rules []Rule, raws []map[string]any) (*Rules, error) {
	if len(rules) == 0 {
		return nil, errors.New("危险命令检测规则为空\n出处：规则文件里一段 [[rules]] 都没有\n出路：恢复出厂的规则文件，或至少写一条 [[rules]]")
	}

	enabledCount := 0
	out := &Rules{}
	for idx, rule := range rules {
		var fields map[string]any
		if idx < len(raws) {
			fields = raws[idx]
		}
		label := ruleLabel(idx, rule, fields)

		// 停用的规则只校验 name（定案 N）：其余字段可空、可非法、正则不必能编译。
		if !rule.enabled() {
			if strings.TrimSpace(rule.Name) == "" {
				return nil, fmt.Errorf(
					"第 %d 条规则被停用（enabled = false），但没有写 name\n出处：这一段 [[rules]] 的 name 字段\n出路：补上 name，例如 name = '递归删除'；不想保留这条规则就整段删掉",
					idx+1)
			}
			out.items = append(out.items, rule)
			out.compiled = append(out.compiled, nil)
			continue
		}
		enabledCount++

		if err := validateRuleText(label, "example", rule.Example); err != nil {
			return nil, err
		}
		if err := validateRuleText(label, "reason", rule.Reason); err != nil {
			return nil, err
		}
		if _, ok := riskOrder[rule.RiskLevel]; !ok {
			return nil, fmt.Errorf(
				"%s 的 risk_level 写成了 '%s'，不是合法等级\n出处：risk_level 字段\n出路：改成 forbidden、high、medium、low 之一，例如 risk_level = 'high'",
				label, rule.RiskLevel)
		}
		if len(rule.Regex) == 0 {
			if fields != nil {
				if _, bad := fields["regex"].(string); bad {
					return nil, fmt.Errorf(
						"%s 的 regex 写成了字符串，规则要求写成数组\n出处：regex 字段\n出路：用方括号包起来，单条也写成数组，例如 regex = ['\\brm -rf /etc\\b']",
						label)
				}
			}
			return nil, fmt.Errorf(
				"%s 没有写 regex\n出处：regex 字段\n出路：写成数组，例如 regex = ['\\brm -rf /etc\\b']",
				label)
		}

		patterns := make([]*regexp.Regexp, 0, len(rule.Regex))
		for i, expr := range rule.Regex {
			if strings.TrimSpace(expr) == "" {
				return nil, fmt.Errorf(
					"%s 的第 %d 条 regex 是空的\n出处：regex 数组的第 %d 项\n出路：填上正则，或把这一项删掉",
					label, i+1, i+1)
			}
			pattern, err := regexp.Compile("(?i)" + expr) // 对位旧 re.IGNORECASE
			if err != nil {
				return nil, fmt.Errorf(
					"%s 的第 %d 条 regex 无法编译：%v\n出处：regex 数组的第 %d 项：'%s'\n出路：Go 正则不支持 (?=...) / (?!...) 这类写法，改成「整行锚定 + 字符类排除」",
					label, i+1, err, i+1, expr)
			}
			patterns = append(patterns, pattern)
		}
		out.items = append(out.items, rule)
		out.compiled = append(out.compiled, patterns)
	}
	if enabledCount == 0 {
		return nil, errors.New(
			"危险命令检测没有任何一条启用的规则，检测形同虚设\n出处：所有 [[rules]] 的 enabled 都是 false\n出路：至少把一条改成 enabled = true，或整段删掉不用的规则")
	}
	return out, nil
}

// decodeHint 把 TOML 解码错误翻成「下一步怎么办」。regex 的类型写错最常见：
// 规则要求数组，写成字符串时解码器只会说「类型不合」，使用者看不出该改哪里。
func decodeHint(err error) string {
	var perr *toml.ParseError
	_ = perr
	msg := err.Error()
	if strings.Contains(msg, "TomlDecodeError") {
		return "出处：检查文件里有没有漏写的引号、写错的方括号"
	}
	if strings.Contains(msg, `"rules.regex"`) && strings.Contains(msg, "string") {
		return "出处：某条规则的 regex 字段\n出路：regex 必须写成数组，单条也写成数组，例如 regex = ['\\brm -rf /etc\\b']"
	}
	if strings.Contains(msg, "TOML value has type") {
		return "出处：报错行里的那个字段\n出路：按规则文件的六项字段核对写法（regex 必须是数组）"
	}
	return "出处：检查文件里有没有漏写的引号、写错的方括号"
}

// ruleLabel 报错里怎么称呼这条规则：有 name 用 name，没有就说第几条。
func ruleLabel(idx int, rule Rule, fields map[string]any) string {
	if name := strings.TrimSpace(rule.Name); name != "" {
		return fmt.Sprintf("规则「%s」", name)
	}
	return fmt.Sprintf("第 %d 条规则", idx+1)
}

// validateRuleText 校验会显示在警告框里的说明类字段：必填、且不超过显示列数上限。
func validateRuleText(label, field, value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return fmt.Errorf(
			"%s 没有写 %s\n出处：%s 字段\n出路：补一句会显示在警告框里的话，例如 %s",
			label, field, field, textFieldHint(field))
	}
	if width := common.DisplayWidth(value); width > ruleTextLimit {
		return fmt.Errorf(
			"%s 的 %s 有 %d 个显示列，超过上限 %d 列（一个汉字占两列）\n出处：%s 字段：'%s'\n出路：改短到 %d 列以内，例如 %s",
			label, field, width, ruleTextLimit, field, value, ruleTextLimit, textFieldHint(field))
	}
	return nil
}

// textFieldHint 给 example / reason 各一句可照抄的示范。
func textFieldHint(field string) string {
	if field == "example" {
		return "example = 'rm -rf /etc'"
	}
	return "reason = '删掉系统配置目录，整批同时无法恢复'"
}

// hitLines 对一条规则的全部正则做匹配，返回命中的行号（升序去重）。
//
// 规则不得跨行（定案 B）：除了不替使用者加 `(?s)`，这里还把命中位置**裁到单行内**
// 再匹配一次——使用者自己在正则里写了 `(?s)` 或 `[\s\S]` 也跨不过去。
func (r *Rules) hitLines(idx int, text string, li lineIndex) []int {
	if r == nil || idx < 0 || idx >= len(r.compiled) {
		return nil
	}
	var lines []int
	seen := map[int]bool{}
	for _, re := range r.compiled[idx] {
		// 按行切段、逐段匹配：同一段里的命中必在同一行内，规则跨不过去（定案 B）。
		// 行尾补一个换行再匹配，等价于「行尾也算词边界」——规则常以 \s 收尾，
		// 不补的话最后一行永远匹配不上。
		for line := range li {
			segEnd := len(text)
			if line+1 < len(li) {
				segEnd = li[line+1] - 1 // 本行换行符的位置
			}
			seg := text[li[line]:segEnd] + "\n"
			if re.MatchString(seg) {
				n := line + 1
				if !seen[n] {
					seen[n] = true
					lines = append(lines, n)
				}
			}
		}
	}
	if len(lines) == 0 {
		return nil
	}
	sort.Ints(lines)
	return lines
}

// 显示列数按 common.DisplayWidth 的同一套口径算（东亚宽字符两列、零宽字符不占列），
// 不另写一份宽度表。
