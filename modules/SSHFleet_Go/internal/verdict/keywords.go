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

	"sshfleet/internal/common"
)

// Category 一个分类及其关键词（顺序即匹配优先级）。
// Tip 是可选的一句话排查建议：本次出现过该分类时，在统计块里跟着分类名提示一行。
//
// 关键词既可以是远端说的话，也可以是工具自己报的报文（`sshfleet.<环境>.<环节>: …`）——
// 两者在匹配上没有区别（ADR-0021 决定第 3 条）。一条分类写不写关键词、写什么，
// 只由匹配语义决定：没有关键词就是永远匹配不上，它的结果落到"未归类"里显示原文。
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
		return nil, common.ErrUserReason(fmt.Sprintf("读取错误分类关键词失败：%s", path), err.Error())
	}
	var file keywordsFile
	md, err := toml.Decode(string(data), &file)
	if err != nil {
		return nil, common.ErrUserReason(fmt.Sprintf("错误分类关键词解析失败：%s", path), err.Error())
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

// validateCategories 逐类校验：只查 name 非空与关键词里的空项。
//
// 「写了名字却没关键词」不拦：一个分类只要写了 name 就合法——没有关键词就是永远
// 匹配不上、结果落到"未归类"，由使用者自己负责。
func validateCategories(items []Category, label string) error {
	for idx, c := range items {
		if strings.TrimSpace(c.Name) == "" {
			return fmt.Errorf("错误分类关键词%s第 %d 类缺少 name", label, idx+1)
		}
		for _, kw := range c.Keywords {
			if strings.TrimSpace(kw) == "" {
				return fmt.Errorf("错误分类%s第 %d 类的 keywords 里有空项", label, idx+1)
			}
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

// passwordExpiredCategory 「密码过期」这个分类名（GLOSSARY「中止词」：归类「密码过期」是约定）。
//
// 为什么是这里的字面量、而不是别再设一个导出常量：改密的入场信号与代填的中止词都只从这里
// 取关键词，它们要的是**远端那句提示的关键词**，不是"分类名由代码给出"。这个字面量是
// 「按分类名查表」的唯一落点：改那条分类的 keywords，两个用途一起跟着变（这是想要的）；
// 改它的 name，查表会落空、两个用途一起失效——启动期的守卫会当场报错拦下。
const passwordExpiredCategory = "密码过期"

// PasswordExpired 判据表里「密码过期」分类的关键词。改密与代填都只从这里取。
//
// 为什么立一个具名方法，而不是让调用方写 KeywordsOf("密码过期")：这个分类名是**跨包约定**
// ——它既是代填的中止词、又是改密的入场信号，consumer 在 batch 与 main 两处。
// 名字写在调用方时，任何一处改字都会静默取到空表（KeywordsOf 未命中返回 nil，不报错）：
// 守卫以为配置正常、执行侧拿不到入场信号，每台机器干等到超时——正是守卫要拦的那个故障，
// 守卫却和故障一起失灵。收在这里之后，改名的落点只有一处。
func (k *Keywords) PasswordExpired() []string {
	return k.KeywordsOf(passwordExpiredCategory)
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
