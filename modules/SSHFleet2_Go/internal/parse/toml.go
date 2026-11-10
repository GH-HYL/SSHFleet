package parse

import (
	"fmt"
	"strconv"
	"strings"
)

// TOML 的一小块：只认随包六份配置真用到的形态。
//
//   - `[节]` 与 `[[数组表]]`、`[a.b]` 点号节——节名原样收；
//   - `键 = 值`：'…' 或 "…" 字符串、整数、布尔；
//   - 跨行数组（`键 = [` 换行 … `]`）整段收掉——判据表那些 keywords 就是这种；
//   - `#` 注释（引号里的 # 是值）、空行跳过。
//
// 引号外转义、重复键这两样出厂配置里没有，遇到就报错，不猜。
// 全文只此一个词法器：配置与三份判据表都从它出的行里取，不另写一套。

// tomlLine 是词法器的一行：属于哪个节（数组表另标）、键、原文值。
type tomlLine struct {
	section string // 节名，点号节原样（interpreter.script）
	array   bool   // 来自 [[…]]
	key     string
	value   string
}

// tokenize 把一份 TOML 文本切成上面的行。
func tokenize(data []byte) ([]tomlLine, error) {
	text := strings.ReplaceAll(string(data), "\r\n", "\n")
	lines := strings.Split(text, "\n")

	var out []tomlLine
	section := ""
	isArray := false
	// 重复键：同一个表里同一个键第二次出现即报错，不取后值（审计 06-H3）。
	// 数组表按「第几条」分作用域——`[[category]]` 各条目键名当然重复，那不叫冲突。
	seenKeys := map[string]map[string]bool{}
	arraySeq := map[string]int{}
	for i := 0; i < len(lines); i++ {
		trimmed := strings.TrimSpace(stripComment(lines[i]))
		if trimmed == "" {
			continue
		}
		if name, array, ok := sectionName(trimmed); ok {
			if name == "" {
				return nil, fmt.Errorf("第 %d 行的节名是空的", i+1)
			}
			section, isArray = name, array
			if array {
				arraySeq[name]++
			}
			out = append(out, tomlLine{section: section, array: isArray})
			continue
		}
		eq := strings.Index(trimmed, "=")
		if eq < 0 {
			return nil, fmt.Errorf("第 %d 行既不是节也不是键值对：%s", i+1, trimmed)
		}
		key := strings.TrimSpace(trimmed[:eq])
		if key == "" {
			return nil, fmt.Errorf("第 %d 行的键名是空的", i+1)
		}
		if section == "" {
			return nil, fmt.Errorf("第 %d 行的键 %s 出现在任何节之前", i+1, key)
		}
		scope := section
		where := "[" + section + "]"
		if isArray {
			scope = section + "#" + strconv.Itoa(arraySeq[section])
			where = fmt.Sprintf("[[%s]] 第 %d 条", section, arraySeq[section])
		}
		name := unquote(key)
		if seenKeys[scope] == nil {
			seenKeys[scope] = map[string]bool{}
		}
		if seenKeys[scope][name] {
			return nil, fmt.Errorf("第 %d 行的键 %s 在 %s 里写了两次", i+1, name, where)
		}
		seenKeys[scope][name] = true
		value := strings.TrimSpace(trimmed[eq+1:])
		// 跨行数组：按引号外的方括号配平来吃，不能「见到 ] 就收」——正则里的
		// [a-zA-Z] 也带 ]，照那个判会把数组腰斩，后面每一行都成了野行。
		for strings.HasPrefix(value, "[") && !balancedArray(value) && i+1 < len(lines) {
			i++
			value += " " + strings.TrimSpace(stripComment(lines[i]))
		}
		out = append(out, tomlLine{section: section, array: isArray, key: key, value: value})
	}
	return out, nil
}

// balancedArray 判一段以 [ 开头的文本，方括号在引号外是否已经配平。
func balancedArray(v string) bool {
	depth := 0
	var quote rune
	for _, r := range v {
		switch {
		case quote != 0:
			if r == quote {
				quote = 0
			}
		case r == '\'' || r == '"':
			quote = r
		case r == '[':
			depth++
		case r == ']':
			depth--
			if depth == 0 {
				return true
			}
		}
	}
	return depth == 0
}

// sectionName 认节行：`[名]` 与 `[[名]]` 都算节，返回里层名字与「是不是数组表」。
func sectionName(trimmed string) (string, bool, bool) {
	switch {
	case strings.HasPrefix(trimmed, "[[") && strings.HasSuffix(trimmed, "]]"):
		return strings.TrimSpace(trimmed[2 : len(trimmed)-2]), true, true
	case strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]"):
		return strings.TrimSpace(trimmed[1 : len(trimmed)-1]), false, true
	}
	return "", false, false
}

// stripComment 裁掉引号外的 # 注释：引号里出现 # 是值的一部分（密码里常有）。
func stripComment(line string) string {
	var quote rune
	for i, r := range line {
		switch {
		case quote != 0:
			if r == quote {
				quote = 0
			}
		case r == '\'' || r == '"':
			quote = r
		case r == '#':
			return line[:i]
		}
	}
	return line
}

// unquote 去掉成对的外层引号（'…' 或 "…"），内部原样留着。
func unquote(v string) string {
	if isQuoted(v) {
		return v[1 : len(v)-1]
	}
	return v
}

// isQuoted 判一段原文是不是成对的引号字符串。
//
// 「要不要引号」看值的类型：数字、布尔不带，字符串一律带。取字符串的地方靠这一条判，
// 免得 `password = /tmp/x` 这种裸值被当成字符串收下（老版的 TOML 库当场就报语法错）。
func isQuoted(v string) bool {
	if len(v) < 2 {
		return false
	}
	c := v[0]
	return (c == '\'' || c == '"') && v[len(v)-1] == c
}

// splitArray 把 `[ "a", "b" ]` 拆成元素。只处理字符串数组——判据表里只有这一种。
func splitArray(raw string) []string {
	s := strings.TrimSpace(raw)
	s = strings.TrimPrefix(s, "[")
	s = strings.TrimSuffix(s, "]")
	var out []string
	var cur strings.Builder
	var quote rune
	for _, r := range s {
		switch {
		case quote != 0:
			if r == quote {
				quote = 0
			} else {
				cur.WriteRune(r)
			}
		case r == '\'' || r == '"':
			quote = r
		case r == ',':
			if v := strings.TrimSpace(cur.String()); v != "" {
				out = append(out, v)
			}
			cur.Reset()
		default:
			cur.WriteRune(r)
		}
	}
	if v := strings.TrimSpace(cur.String()); v != "" {
		out = append(out, v)
	}
	return out
}

// rawConfig 配置原文表：节名 → （键名 → 原文值）。只用于单值配置，不用于数组表。
type rawConfig map[string]map[string]string

// rawView 把行摊成原文表：同名节的单值键并进同一张表。
//
// 重复键在这里不会再出现——`tokenize` 已经当场报错退回了（2026-11-10）。写成后值盖
// 前值的时候，这个函数看起来像「允许重复」，实际是让配置写坏也能跑、还退 0。
//
// 值存**原文**（引号留在里面）：字符串要不要引号是取值时按类型判的事（`stringOf`），
// 这里提前剥掉就把「写没写引号」这个事实抹了。
func rawView(lines []tomlLine) rawConfig {
	raw := rawConfig{}
	for _, ln := range lines {
		if raw[ln.section] == nil {
			raw[ln.section] = map[string]string{}
		}
		if ln.key == "" {
			continue
		}
		raw[ln.section][unquote(ln.key)] = ln.value
	}
	return raw
}

// arrayItems 取某个数组表（[[名]]）的条目，按出现顺序；每条的键 → 原文值。
func arrayItems(lines []tomlLine, section string) []map[string]string {
	var items []map[string]string
	for _, ln := range lines {
		if ln.section != section {
			continue
		}
		if ln.key == "" {
			if ln.array {
				items = append(items, map[string]string{})
			}
			continue
		}
		if len(items) == 0 {
			items = append(items, map[string]string{})
		}
		items[len(items)-1][unquote(ln.key)] = ln.value
	}
	return items
}

// arrayOrder 报数组表某个键在这份文件里出现的先后（用于保序判据表）。
func hasArrayTable(lines []tomlLine, section string) bool {
	for _, ln := range lines {
		if ln.section == section && ln.array {
			return true
		}
	}
	return false
}
