// 错误分类判据（TOML，保序）：两块表，触发条件互斥——
//
//	[[categories]]            仅当退出码为 nil（命令未执行）时参与匹配
//	[[exit_code_categories]]  仅当退出码非 0 时参与匹配（命中即覆盖退出码分类，ADR-0004）
//
// 块内自上而下遍历，取第一个命中的分类。通配符只有星号 *（"任意长度的内容"），
// 其余符号按普通字符处理。判据写什么、按什么口径取舍，见 config/error_keywords.toml 头部说明。
package result

import (
	"fmt"
	"os"
	"strings"

	"github.com/BurntSushi/toml"
)

// Category 一个分类及其关键词（顺序即匹配优先级）。
type Category struct {
	Name     string   `toml:"name"`
	Keywords []string `toml:"keywords"`
}

// Keywords 加载后的判据表：两块，触发条件互斥。
type Keywords struct {
	items     []Category // 第一块：退出码为 nil 时用
	exitItems []Category // 第二块：退出码非 0 时用
}

type keywordsFile struct {
	Categories     []Category `toml:"categories"`
	ExitCategories []Category `toml:"exit_code_categories"`
}

// LoadKeywords 读取错误分类判据文件。
//
// 第一块不可为空——连它都没有等于没有分类能力；第二块可为空，空即"没有任何分类
// 能在退出码非 0 时覆盖它"，此时退出码分类照旧（ADR-0004）。
func LoadKeywords(path string) (*Keywords, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取错误分类关键词失败：%s\n原因：%v", path, err)
	}
	var file keywordsFile
	md, err := toml.Decode(string(data), &file)
	if err != nil {
		return nil, fmt.Errorf("错误分类关键词解析失败：%s\n原因：%v", path, err)
	}
	if undecoded := md.Undecoded(); len(undecoded) > 0 {
		keys := make([]string, 0, len(undecoded))
		for _, k := range undecoded {
			keys = append(keys, k.String())
		}
		return nil, fmt.Errorf("错误分类关键词含未识别字段：%s", strings.Join(keys, ", "))
	}
	if len(file.Categories) == 0 {
		return nil, fmt.Errorf("错误分类关键词为空：%s", path)
	}
	if err := validateCategories(file.Categories, ""); err != nil {
		return nil, err
	}
	if err := validateCategories(file.ExitCategories, "（退出码分块）"); err != nil {
		return nil, err
	}
	return &Keywords{items: file.Categories, exitItems: file.ExitCategories}, nil
}

// validateCategories 逐类校验：name 非空、关键词非空。label 用于标明是哪一块出的错。
func validateCategories(items []Category, label string) error {
	for idx, c := range items {
		if strings.TrimSpace(c.Name) == "" {
			return fmt.Errorf("错误分类关键词%s第 %d 类缺少 name", label, idx+1)
		}
		if len(c.Keywords) == 0 {
			return fmt.Errorf("错误分类%s没有任何关键词", label+c.Name)
		}
	}
	return nil
}

// Names 分类名列表（保序，两块并集、同名去重；测试与展示用）。
func (k *Keywords) Names() []string {
	out := make([]string, 0, len(k.items)+len(k.exitItems))
	seen := make(map[string]bool, len(k.items)+len(k.exitItems))
	for _, c := range k.all() {
		if seen[c.Name] {
			continue
		}
		seen[c.Name] = true
		out = append(out, c.Name)
	}
	return out
}

// all 两块按顺序拼起来（只用于遍历，不持有）。
func (k *Keywords) all() []Category {
	out := make([]Category, 0, len(k.items)+len(k.exitItems))
	out = append(out, k.items...)
	out = append(out, k.exitItems...)
	return out
}

// Has 分类名是否存在。**必须覆盖两块**：IsFallbackCategory 靠它区分"已知分类"
// 与"判据未命中时的兜底原文"，漏掉第二块会把覆盖出来的分类误判成兜底原文。
func (k *Keywords) Has(name string) bool {
	for _, c := range k.items {
		if c.Name == name {
			return true
		}
	}
	for _, c := range k.exitItems {
		if c.Name == name {
			return true
		}
	}
	return false
}

// KeywordsOf 取某分类的关键词（不存在返回 nil）。同名分类在两块各有一份判据，这里取并集。
func (k *Keywords) KeywordsOf(name string) []string {
	var out []string
	for _, c := range k.items {
		if c.Name == name {
			out = append(out, c.Keywords...)
		}
	}
	for _, c := range k.exitItems {
		if c.Name == name {
			out = append(out, c.Keywords...)
		}
	}
	return out
}

// match 第一块匹配（退出码为 nil 时用），返回第一个命中的分类；未命中返回空串。
// 判据表可能为 nil（调用方允许不加载判据），此时一律不命中。
func (k *Keywords) match(text string) string {
	if k == nil {
		return ""
	}
	return matchIn(k.items, text)
}

// matchExitCode 第二块匹配（退出码非 0 时用）：命中即覆盖退出码分类（ADR-0004）。
func (k *Keywords) matchExitCode(text string) string {
	if k == nil {
		return ""
	}
	return matchIn(k.exitItems, text)
}

// matchIn 块内匹配：自上而下遍历分类，取第一个命中的。
func matchIn(items []Category, text string) string {
	if len(items) == 0 || text == "" {
		return ""
	}
	lower := strings.ToLower(text)
	for _, c := range items {
		for _, kw := range c.Keywords {
			if keywordHit(strings.ToLower(kw), lower) {
				return c.Name
			}
		}
	}
	return ""
}

// keywordHit 单条关键词命中判定：按 * 切片段后顺序查找；不含 * 即普通子串包含。
func keywordHit(keyword, text string) bool {
	if !strings.Contains(keyword, "*") {
		return strings.Contains(text, keyword)
	}
	pos := 0
	for _, part := range strings.Split(keyword, "*") {
		if part == "" {
			continue
		}
		idx := strings.Index(text[pos:], part)
		if idx < 0 {
			return false
		}
		pos += idx + len(part)
	}
	return true
}
