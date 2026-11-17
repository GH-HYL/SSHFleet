// 纯文本整备：判定前对命令原文做的两步处理，都只动字面、不建命令结构。
//
// 为什么要有这一层：判定对象是「命令原文」，但原文里有几类写法会平白制造误报或漏报——
// 注释里的危险词、`//x` 这样的斜杠变体。整备把它们按字面抹平，规则就能写短。
//
// 两条铁律：
//   - 输出与输入**行数相同**。行号是给使用者的坐标，整备一压缩行，坐标就错了。
//   - 不解析、不分词、不认语言。只做「扫一遍、按位置改写」。
package dangercheck

import (
	"strings"
)

// prepare 把原文整备成用于匹配的文本（定案 D：与下发侧同一份正文，见 check.go）。
//
// 顺序：去注释 → 路径片段归一。两步都保持行数与换行位置不变。
func prepare(src string) string {
	return normalizePathFragments(stripComments(src))
}

// stripComments 整行注释、行尾注释与纯空白行不参与匹配（方案第 3 条）。
//
// 做法是**把注释换成空格**而不是删行或删列：行数不变，且行内其余内容的列位置也不变。
// 下面三类 `#` 不是注释起点，必须保留：
//   - 引号里的 `#`（`echo "issue #123"` 里的 `#` 是内容）
//   - `$#`（位置参数个数）
//   - 紧贴字母或数字的 `#`（`http://x/#a` 里的路径片段）
//
// 逐行判断，行内的 `#` 只认第一个真正的注释起点——注释之后到行尾全部是注释。
func stripComments(src string) string {
	lines := strings.Split(src, "\n")
	for i, ln := range lines {
		lines[i] = stripLineComment(ln)
	}
	return strings.Join(lines, "\n")
}

// stripLineComment 抹掉一行的注释部分（含整行注释），用空格占位。
func stripLineComment(line string) string {
	cut := -1 // 注释起点；-1 表示整行没有注释
	var quote rune
	prev := rune(0)
	for i, r := range line {
		switch {
		case quote != 0:
			if r == quote {
				quote = 0
			}
		case r == '\'' || r == '"':
			quote = r
		case r == '#' && prev != '$' && !isWordRune(prev):
			// 前面是空白、行首或标点时 `#` 才是注释起点；紧贴字母/数字的是路径片段。
			cut = i
		}
		if cut >= 0 {
			break
		}
		prev = r
	}
	if cut < 0 {
		return line
	}
	return line[:cut] + strings.Repeat(" ", len(line)-cut)
}

// isWordRune 紧贴在 `#` 前面时说明它是内容的一部分（字母、数字、下划线）。
func isWordRune(r rune) bool {
	return r == '_' ||
		(r >= 'a' && r <= 'z') ||
		(r >= 'A' && r <= 'Z') ||
		(r >= '0' && r <= '9') ||
		r > 127 // 非 ASCII 字符一律当内容，宁可少剔也不误剔
}

// pathRuns 在一行里找形如 `/a/b` 的路径片段，返回每段的起止字节位置。
//
// 只认「以 / 开头、由路径字符组成」的连续片段；不作为分词依据，只用来决定哪些
// 字节串可以就地整备。
func pathRuns(line string) [][2]int {
	var runs [][2]int
	i := 0
	for i < len(line) {
		if line[i] != '/' {
			i++
			continue
		}
		start := i
		j := i + 1
		for j < len(line) && isPathByte(line[j]) {
			j++
		}
		runs = append(runs, [2]int{start, j})
		i = j
	}
	return runs
}

// isPathByte 路径片段里允许出现的字节。
func isPathByte(b byte) bool {
	switch {
	case b >= 'a' && b <= 'z', b >= 'A' && b <= 'Z', b >= '0' && b <= '9':
		return true
	}
	switch b {
	case '/', '.', '-', '_', '*', '?', '~', '@', '+', '%', '=', ':':
		return true
	}
	return false
}

// normalizePathFragments 路径归一（定案 C 保留的那一项）：把文本里的路径片段按字面整备——
// 合并重复斜杠、去尾斜杠、词法解掉 `.` 与 `..`。替换是等长范围内的就地改写，
// 不增删换行，所以行号口径不受影响。
func normalizePathFragments(src string) string {
	var b strings.Builder
	b.Grow(len(src))
	for _, line := range strings.SplitAfter(src, "\n") {
		eol := ""
		body := line
		if strings.HasSuffix(body, "\n") {
			eol = "\n"
			body = body[:len(body)-1]
		}
		b.WriteString(normalizeLinePaths(body))
		// 行尾补一个空格：规则常以 `\s` 收尾（「路径后面是一个词边界」），
		// 不补的话「路径恰好写在行尾」永远命中不了。只补空格不补换行，行数不变。
		if eol != "" {
			b.WriteString(" ")
		}
		b.WriteString(eol)
	}
	return b.String()
}

// normalizeLinePaths 对一行里的每个路径片段做整备。
func normalizeLinePaths(line string) string {
	runs := pathRuns(line)
	if len(runs) == 0 {
		return line
	}
	var b strings.Builder
	last := 0
	for _, r := range runs {
		b.WriteString(line[last:r[0]])
		b.WriteString(cleanPath(line[r[0]:r[1]]))
		last = r[1]
	}
	b.WriteString(line[last:])
	return b.String()
}

// cleanPath 单个路径片段的词法整备。`*` / `?` 这类通配符按普通字符对待，原样保留。
func cleanPath(p string) string {
	for strings.Contains(p, "//") {
		p = strings.ReplaceAll(p, "//", "/")
	}
	parts := make([]string, 0, 4)
	for _, seg := range strings.Split(p[1:], "/") {
		switch seg {
		case "", ".":
			continue
		case "..":
			if len(parts) > 0 {
				parts = parts[:len(parts)-1]
			}
		default:
			parts = append(parts, seg)
		}
	}
	if len(parts) == 0 {
		return "/"
	}
	return "/" + strings.Join(parts, "/")
}
