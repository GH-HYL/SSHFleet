package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/BurntSushi/toml"
)

// 批量改密（--change-password）专用配置：一个文件装两类文字——提示词表与失败分类。
//
// 为什么由本层统一解析：这两类文字分别给会话匹配器（internal/ssh）与错误分类
// （internal/result）用，而两个包都做严格字段校验（不认识的字段即报错）。同一份文件
// 被两个包各解析一遍的话，各自都会把对方那段当成「未识别字段」。在本层读一次、
// 各自取自己那部分，严格校验照样保得住。

// PasswdStep 一条提示：提示原文 + 该喂哪个值 + 最多喂几次。
type PasswdStep struct {
	Keywords []string `toml:"keywords"`
	Value    string   `toml:"value"` // current / new
	Max      int      `toml:"max"`
}

// PasswdCategory 一条失败分类：按远端说的原话归类。
type PasswdCategory struct {
	Name     string   `toml:"name"`
	Keywords []string `toml:"keywords"`
	Tip      string   `toml:"tip"`
}

// PasswdConfig 改密配置的内容。
type PasswdConfig struct {
	Steps      []PasswdStep     `toml:"step"`
	Categories []PasswdCategory `toml:"category"`
}

// LoadPasswdConfig 读取并校验改密配置。
//
// 提示词表缺一即报错——它是改密唯一的「认路」依据，缺了只能干等到超时。
// 失败分类可以为空：空即改密失败一律落到通用判据表。
func LoadPasswdConfig(path string) (*PasswdConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取改密配置失败：%s\n原因：%v", path, err)
	}
	var file PasswdConfig
	md, err := toml.Decode(string(data), &file)
	if err != nil {
		return nil, fmt.Errorf("改密配置解析失败：%s\n原因：%v", path, err)
	}
	if undecoded := md.Undecoded(); len(undecoded) > 0 {
		keys := make([]string, 0, len(undecoded))
		for _, k := range undecoded {
			keys = append(keys, k.String())
		}
		return nil, fmt.Errorf("改密配置含未识别字段：%s", strings.Join(keys, ", "))
	}
	if len(file.Steps) == 0 {
		return nil, fmt.Errorf("改密配置里一条 step 都没有：%s\n提示：至少留一条 [[step]]", path)
	}
	for i, step := range file.Steps {
		if err := step.validate(); err != nil {
			return nil, fmt.Errorf("改密配置第 %d 条 step：%v", i+1, err)
		}
	}
	for i, c := range file.Categories {
		if err := c.validate(); err != nil {
			return nil, fmt.Errorf("改密配置第 %d 条 category：%v", i+1, err)
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
	// 值与 max 都与执行侧的两个枚举/边界对齐，写错在启动时就拦下
	switch s.Value {
	case "current", "new":
	default:
		return fmt.Errorf("value 只能写 current 或 new，当前值：%s", s.Value)
	}
	if s.Max < 1 {
		return fmt.Errorf("max 必须是正整数，当前值：%d", s.Max)
	}
	return nil
}

// validate 单条 category 的校验。
func (c PasswdCategory) validate() error {
	if strings.TrimSpace(c.Name) == "" {
		return fmt.Errorf("缺少 name（分类名）")
	}
	if len(c.Keywords) == 0 {
		return fmt.Errorf("%s 没有任何关键词", c.Name)
	}
	for _, kw := range c.Keywords {
		if strings.TrimSpace(kw) == "" {
			return fmt.Errorf("%s 的 keywords 里有空项", c.Name)
		}
	}
	return nil
}
