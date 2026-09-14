// 错误分类关键词（TOML，保序）：自上而下遍历，取第一个命中的分类。
// 通配符只有星号 *（"任意长度的内容"），其余符号按普通字符处理（spec D11 语义不动）。
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

// Keywords 加载后的关键词表。
type Keywords struct {
	items []Category
}

type keywordsFile struct {
	Categories []Category `toml:"categories"`
}

// LoadKeywords 读取错误分类关键词文件。
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
	for idx, c := range file.Categories {
		if strings.TrimSpace(c.Name) == "" {
			return nil, fmt.Errorf("错误分类关键词第 %d 类缺少 name", idx+1)
		}
		if len(c.Keywords) == 0 {
			return nil, fmt.Errorf("错误分类 %s 没有任何关键词", c.Name)
		}
	}
	return &Keywords{items: file.Categories}, nil
}

// Names 分类名列表（保序，测试与展示用）。
func (k *Keywords) Names() []string {
	out := make([]string, 0, len(k.items))
	for _, c := range k.items {
		out = append(out, c.Name)
	}
	return out
}

// Has 分类名是否存在。
func (k *Keywords) Has(name string) bool {
	for _, c := range k.items {
		if c.Name == name {
			return true
		}
	}
	return false
}

// KeywordsOf 取某分类的关键词（不存在返回 nil）。
func (k *Keywords) KeywordsOf(name string) []string {
	for _, c := range k.items {
		if c.Name == name {
			return c.Keywords
		}
	}
	return nil
}

// match 关键词匹配，返回第一个命中的分类；未命中返回空串。
func (k *Keywords) match(text string) string {
	if k == nil || text == "" {
		return ""
	}
	lower := strings.ToLower(text)
	for _, c := range k.items {
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
