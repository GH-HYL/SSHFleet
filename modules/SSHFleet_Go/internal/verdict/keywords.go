// 错误分类判据（TOML，保序）：两块表，选组依据见 verdict.Judge（result-verdict spec 4.3）——
//
//	[[categories]]            第一组：目的那条命令没有给出退出码时用（连不上、传输失败、改密……）
//	[[exit_code_categories]]  第二组：目的那条命令自己以非 0 收场时用（会话被拒 / 命令自身失败）
//
// 块内自上而下遍历，取第一个命中的分类。通配符只有星号 *（"任意长度的内容"），
// 其余符号按普通字符处理。判据写什么、按什么口径取舍，见 config/keywords_error.conf 头部说明。
package verdict

import (
	"fmt"
	"os"
	"strings"

	"github.com/BurntSushi/toml"
)

// Category 一个分类及其关键词（顺序即匹配优先级）。
// Tip 是可选的一句话排查建议：本次出现过该分类时，在统计块里跟着分类名提示一行。
type Category struct {
	Name     string   `toml:"name"`
	Keywords []string `toml:"keywords"`
	Tip      string   `toml:"tip"`
}

// Keywords 加载后的判据表：两块，选组依据是"目的命令有没有给出退出码"（4.3）。
type Keywords struct {
	items     []Category // 第一组
	exitItems []Category // 第二组
}

type keywordsFile struct {
	Categories     []Category `toml:"categories"`
	ExitCategories []Category `toml:"exit_code_categories"`
}

// LoadKeywords 读取错误分类判据文件。
//
// 第一组不可为空——连它都没有等于没有分类能力；第二组可为空，空即"没有任何分类
// 能在目的命令给出非 0 退出码时接住它"，此时按退出码兜底分类照旧。
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

// PrependCategories 在判据表最前面插入一组分类（改密专用分类走这里进来）。
//
// 插在最前面，因为改密的机器本来就带着「密码过期」那句服务端提示，而那批关键词在通用表
// 中间——不插到前面就会被抢走分类（2026-09-29 实测，见 config/passwd.conf 头部说明）。
func (k *Keywords) PrependCategories(items []Category) {
	if k == nil || len(items) == 0 {
		return
	}
	merged := make([]Category, 0, len(items)+len(k.items))
	merged = append(merged, items...)
	k.items = append(merged, k.items...)
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
// 与"判据未命中时的兜底原文"，漏掉第二组会把选组出来的分类误判成兜底原文。
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

// TipOf 取某分类的提示语（统计块里跟着分类名显示的那句排查建议）。
// 没写 tip 的分类返回空串。同名分类跨两块时，取先出现且非空的那条。
func (k *Keywords) TipOf(name string) string {
	if k == nil {
		return ""
	}
	for _, c := range k.items {
		if c.Name == name && c.Tip != "" {
			return c.Tip
		}
	}
	for _, c := range k.exitItems {
		if c.Name == name && c.Tip != "" {
			return c.Tip
		}
	}
	return ""
}

// matchText 按组匹配文本（选组由 Judge 按 4.3 的判据决定，本函数不做选组）：
// second 为 true 查第二组，否则查第一组。返回第一个命中的分类；未命中返回空串。
// 判据表可能为 nil（调用方允许不加载判据），此时一律不命中。
func (k *Keywords) matchText(second bool, text string) string {
	if k == nil {
		return ""
	}
	if second {
		return matchIn(k.exitItems, text)
	}
	return matchIn(k.items, text)
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
