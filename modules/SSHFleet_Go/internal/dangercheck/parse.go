// 解析层（spec D25）：用 mvdan.cc/sh/v3/syntax 把命令文本解析成命令段。
// 只做结构解析，不做风险判定——判定见 judge.go。
//
// 与旧实现的差异（D33）：脚本检测先整体解析成语法树，解析失败（脚本有语法错）
// 再回退逐行——旧实现一律逐行解析，续行符、heredoc 会把一条命令割裂。
// 判定层（包装命令剥离、rm 旗标归一、间接执行递归）仍是自己实现，不依赖库。
package dangercheck

import (
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

// 内置语法清单（shell 机制，与用户规则无关）
var (
	// wrappers 命令前可叠加的包装命令（剥除后命令词才是真实命令）
	wrappers = map[string]bool{
		"sudo": true, "time": true, "nohup": true, "setsid": true,
		"stdbuf": true, "xargs": true, "env": true, "command": true,
	}
	// wrapperOptions 包装命令中「带值」的选项（剥除时连选项值一起跳过）
	wrapperOptions = map[string]map[string]bool{
		"sudo":  {"-u": true, "-g": true, "-p": true, "-C": true, "--user": true, "--group": true, "--prompt": true},
		"xargs": {"-I": true, "-n": true, "-P": true, "-s": true, "-a": true, "-d": true, "--delimiter": true, "--max-args": true, "--max-chars": true},
	}
	shellCommands  = map[string]bool{"bash": true, "sh": true, "zsh": true, "dash": true, "ksh": true}
	evalCommands   = map[string]bool{"eval": true}
	remoteCommands = map[string]bool{"ssh": true}
	// remoteValueOpts ssh 中带值的选项
	remoteValueOpts = map[string]bool{"-p": true, "-l": true, "-F": true, "-o": true, "-i": true, "-J": true, "-L": true, "-R": true, "-W": true, "-b": true, "-e": true, "-c": true}
)

const maxDepth = 5

// Segment 待判定的命令段：以真实命令词开头（命令词已归一为不含目录的形式）。
type Segment struct {
	Tokens []string
	Text   string
	Line   int
}

// ParseSource 解析一段命令文本（命令模式为一行，脚本模式为全文）。
func ParseSource(src string) []Segment {
	if segs, ok := parseSource(src); ok {
		return segs
	}
	return parseFallback(src)
}

// parseSource 语法树解析：成功返回命令段，语法错误返回 ok=false（交给调用方回退）。
func parseSource(src string) ([]Segment, bool) {
	parser := syntax.NewParser()
	file, err := parser.Parse(strings.NewReader(src), "")
	if err != nil {
		return nil, false
	}

	var out []Segment
	collectStmts(file.Stmts, 0, &out)
	return out, true
}

// collectStmts 遍历语句列表，递归处理管道/复合语句/函数体。
func collectStmts(stmts []*syntax.Stmt, depth int, out *[]Segment) {
	if depth > maxDepth {
		return
	}
	for _, st := range stmts {
		collectCmd(st.Cmd, depth, out)
	}
}

func collectCmd(cmd syntax.Command, depth int, out *[]Segment) {
	if cmd == nil || depth > maxDepth {
		return
	}
	switch c := cmd.(type) {
	case *syntax.CallExpr:
		seg := renderCall(c, depth, out)
		if seg != nil {
			*out = append(*out, *seg)
		}
	case *syntax.BinaryCmd:
		collectStmts([]*syntax.Stmt{c.X}, depth, out)
		collectStmts([]*syntax.Stmt{c.Y}, depth, out)
	case *syntax.Block:
		collectStmts(c.Stmts, depth+1, out)
	case *syntax.Subshell:
		collectStmts(c.Stmts, depth+1, out)
	case *syntax.IfClause:
		collectStmts(c.Cond, depth+1, out)
		collectStmts(c.Then, depth+1, out)
		if c.Else != nil {
			collectCmd(c.Else, depth+1, out)
		}
	case *syntax.WhileClause:
		collectStmts(c.Cond, depth+1, out)
		collectStmts(c.Do, depth+1, out)
	case *syntax.ForClause:
		collectStmts(c.Do, depth+1, out)
	case *syntax.CaseClause:
		for _, item := range c.Items {
			collectStmts(item.Stmts, depth+1, out)
		}
	case *syntax.FuncDecl:
		if c.Body != nil {
			collectCmd(c.Body.Cmd, depth+1, out)
		}
	case *syntax.ArithmCmd, *syntax.TestClause, *syntax.DeclClause, *syntax.LetClause, *syntax.TimeClause, *syntax.CoprocClause:
		if t, ok := cmd.(*syntax.TimeClause); ok && t.Stmt != nil {
			collectCmd(t.Stmt.Cmd, depth+1, out)
		}
	}
}

// renderCall 渲染一次调用为命令段：剥环境变量赋值与前导包装命令、命令词归一，
// 并对间接执行（shell -c / eval / ssh 远端）递归取内部命令。
func renderCall(c *syntax.CallExpr, depth int, out *[]Segment) *Segment {
	rendered := make([]string, 0, len(c.Args))
	for _, w := range c.Args {
		rendered = append(rendered, renderWord(w, depth, out))
	}
	if len(rendered) == 0 {
		return nil
	}

	tokens := stripPrefixes(rendered)
	if len(tokens) == 0 {
		return nil
	}

	// 命令词归一：/bin/rm → rm，./rm → rm
	base := tokens[0]
	if i := strings.LastIndexByte(base, '/'); i >= 0 {
		base = base[i+1:]
	}
	normalized := append([]string{base}, tokens[1:]...)
	line := int(c.Pos().Line())

	// 间接执行：内部命令字符串递归解析
	if depth < maxDepth {
		for _, inner := range extractInnerCommands(base, tokens[1:]) {
			for _, s := range ParseSource(inner) {
				*out = append(*out, s)
			}
		}
	}

	return &Segment{Tokens: normalized, Text: strings.Join(normalized, " "), Line: line}
}

// renderWord 把语法树的词渲染成用于匹配的纯文本：
// 命令替换（$(...) 与反引号）以占位符表示，其内部命令**另行递归解析**成独立命令段
// （对位旧实现把替换内容抽出后单独分析）。
func renderWord(w *syntax.Word, depth int, out *[]Segment) string {
	var b strings.Builder
	for _, part := range w.Parts {
		switch p := part.(type) {
		case *syntax.Lit:
			b.WriteString(p.Value)
		case *syntax.SglQuoted:
			b.WriteString(p.Value)
		case *syntax.DblQuoted:
			for _, inner := range p.Parts {
				b.WriteString(renderPart(inner, depth, out))
			}
		default:
			b.WriteString(renderPart(part, depth, out))
		}
	}
	return b.String()
}

func renderPart(part syntax.WordPart, depth int, out *[]Segment) string {
	switch p := part.(type) {
	case *syntax.Lit:
		return p.Value
	case *syntax.SglQuoted:
		return p.Value
	case *syntax.DblQuoted:
		var b strings.Builder
		for _, inner := range p.Parts {
			b.WriteString(renderPart(inner, depth, out))
		}
		return b.String()
	case *syntax.ParamExp:
		return "$" + p.Param.Value
	case *syntax.CmdSubst:
		// 命令替换内部真实执行：递归解析（受深度上限约束）
		if depth < maxDepth {
			collectStmts(p.Stmts, depth+1, out)
		}
		return "$(...)"
	case *syntax.ArithmExp:
		return "$((...))"
	case *syntax.ProcSubst:
		if depth < maxDepth {
			collectStmts(p.Stmts, depth+1, out)
		}
		return "<(...)"
	case *syntax.ExtGlob:
		return p.Pattern.Value
	default:
		return ""
	}
}

// extractInnerCommands 提取间接执行的内部命令字符串（shell -c / eval / ssh 远端）。
func extractInnerCommands(cmd string, rest []string) []string {
	var inner []string
	switch {
	case shellCommands[cmd]:
		for i, tok := range rest {
			if tok == "-c" && i+1 < len(rest) {
				inner = append(inner, strings.Join(rest[i+1:], " "))
				break
			}
		}
	case evalCommands[cmd]:
		if len(rest) > 0 {
			inner = append(inner, strings.Join(rest, " "))
		}
	case remoteCommands[cmd]:
		i := 0
		for i < len(rest) && strings.HasPrefix(rest[i], "-") && rest[i] != "-" {
			if remoteValueOpts[rest[i]] && i+1 < len(rest) {
				i += 2
			} else {
				i++
			}
		}
		args := rest[i:]
		if len(args) > 1 {
			remote := strings.TrimSpace(strings.Join(args[1:], " "))
			if remote != "" {
				inner = append(inner, remote)
			}
		}
	}
	filtered := inner[:0]
	for _, x := range inner {
		if strings.TrimSpace(x) != "" {
			filtered = append(filtered, x)
		}
	}
	return filtered
}

// stripPrefixes 剥除环境变量赋值与前导包装命令，返回从真实命令词开始的 token。
func stripPrefixes(tokens []string) []string {
	i := 0
	for i < len(tokens) {
		tok := tokens[i]
		if isAssign(tok) {
			i++
			continue
		}
		if wrappers[tok] {
			i++
			valueOpts := wrapperOptions[tok]
			for i < len(tokens) {
				nxt := tokens[i]
				if strings.HasPrefix(nxt, "-") && nxt != "-" {
					if valueOpts[nxt] {
						i += 2
					} else {
						i++
					}
					continue
				}
				break
			}
			continue
		}
		break
	}
	if i > len(tokens) {
		i = len(tokens)
	}
	return tokens[i:]
}

// isAssign 环境变量赋值形式 NAME=value（对位旧实现的正则）。
func isAssign(tok string) bool {
	eq := strings.IndexByte(tok, '=')
	if eq <= 0 {
		return false
	}
	for i, r := range tok[:eq] {
		if i == 0 {
			if !(r == '_' || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')) {
				return false
			}
			continue
		}
		if !(r == '_' || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')) {
			return false
		}
	}
	return true
}

// parseFallback 语法树解析失败时的回退：逐行解析（跳过空行与注释行）。
func parseFallback(src string) []Segment {
	var out []Segment
	for idx, line := range strings.Split(src, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if segs, ok := parseSource(trimmed); ok {
			for _, s := range segs {
				s.Line = idx + 1
				out = append(out, s)
			}
			continue
		}
		// 连逐行解析都不行：朴素分词（引号内合并），保证仍能判定
		tokens := naiveTokenize(trimmed)
		if len(tokens) == 0 {
			continue
		}
		base := tokens[0]
		if i := strings.LastIndexByte(base, '/'); i >= 0 {
			base = base[i+1:]
		}
		normalized := append([]string{base}, tokens[1:]...)
		out = append(out, Segment{Tokens: normalized, Text: strings.Join(normalized, " "), Line: idx + 1})
	}
	return out
}

// naiveTokenize 朴素分词：空白切分，引号包裹内容合并为单 token。
func naiveTokenize(line string) []string {
	var (
		tokens []string
		buf    strings.Builder
		quote  rune
	)
	flush := func() {
		if buf.Len() > 0 {
			tokens = append(tokens, buf.String())
			buf.Reset()
		}
	}
	for _, r := range line {
		switch {
		case quote != 0:
			if r == quote {
				quote = 0
			} else {
				buf.WriteRune(r)
			}
		case r == '\'' || r == '"':
			quote = r
		case r == ' ' || r == '\t':
			flush()
		case r == ';' || r == '|' || r == '&':
			flush()
		default:
			buf.WriteRune(r)
		}
	}
	flush()
	return tokens
}
