package ssh

import (
	"fmt"
	"os"
	"strings"

	"github.com/BurntSushi/toml"
)

// 批量改密（--change-password）的提示词表：认出远端在问什么，就喂对应的值。
//
// 值不在这张表里——当前密码取清单里那台机器的登录密码，新密码取命令行参数。
// 表只回答两件事：哪一段文字是提问，这一问该喂哪个值。

// PasswdValue 一条 step 该喂的值从哪来。只能是下面两个取值之一。
type PasswdValue string

const (
	// PasswdValueCurrent 喂清单里那台机器的登录密码。
	PasswdValueCurrent PasswdValue = "current"
	// PasswdValueNew 喂 --change-password 给的新密码。
	PasswdValueNew PasswdValue = "new"
)

// PasswdStep 一条提示：提示原文 + 该喂哪个值 + 最多喂几次。
type PasswdStep struct {
	Keywords []string    `toml:"keywords"`
	Value    PasswdValue `toml:"value"`
	Max      int         `toml:"max"`
}

// PasswdPrompts 提示词表。顺序即提问顺序。
type PasswdPrompts struct {
	Steps []PasswdStep `toml:"step"`
}

// LoadPasswdPrompts 读取并校验提示词表。
//
// 字段缺一即报错：这份表是改密唯一的"认路"依据，缺了它只能干等到超时。
func LoadPasswdPrompts(path string) (*PasswdPrompts, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取改密提示词表失败：%s\n原因：%v", path, err)
	}
	var file PasswdPrompts
	md, err := toml.Decode(string(data), &file)
	if err != nil {
		return nil, fmt.Errorf("改密提示词表解析失败：%s\n原因：%v", path, err)
	}
	if undecoded := md.Undecoded(); len(undecoded) > 0 {
		keys := make([]string, 0, len(undecoded))
		for _, k := range undecoded {
			keys = append(keys, k.String())
		}
		return nil, fmt.Errorf("改密提示词表含未识别字段：%s", strings.Join(keys, ", "))
	}
	if len(file.Steps) == 0 {
		return nil, fmt.Errorf("改密提示词表里一条 step 都没有：%s\n提示：至少留一条 [[step]]", path)
	}
	for i, step := range file.Steps {
		if err := step.validate(); err != nil {
			return nil, fmt.Errorf("改密提示词表第 %d 条：%v", i+1, err)
		}
	}
	return &file, nil
}

// validate 单条 step 的校验。
func (s PasswdStep) validate() error {
	if len(s.Keywords) == 0 {
		return fmt.Errorf("缺少 keywords（提示原文）")
	}
	for _, kw := range s.Keywords {
		if strings.TrimSpace(kw) == "" {
			return fmt.Errorf("keywords 里有空项")
		}
	}
	switch s.Value {
	case PasswdValueCurrent, PasswdValueNew:
	default:
		return fmt.Errorf("value 只能写 current 或 new，当前值：%s", s.Value)
	}
	if s.Max < 1 {
		return fmt.Errorf("max 必须是正整数，当前值：%d", s.Max)
	}
	return nil
}

// stepBudget 全表最多会喂多少次（送信通道的容量按它给，保证钩子永不阻塞）。
func (p *PasswdPrompts) stepBudget() int {
	total := 0
	for _, s := range p.Steps {
		total += s.Max
	}
	return total
}
