package guard

import (
	"fmt"
	"go/ast"
	"go/token"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

// 环节名对照判据表：代码里产出的每个「环境.环节」，随包的判据表里都要有一条。
//
// 方向**单向**（ADR-0021 决定 8 条那一段）：改环节名要同步改表，漏改那一类就回落成报错原文，
// 使用者只会看到「未归类」。反向不判——判据表里有些条目由别的站产出，只看这一处看不全。
//
// 借类二的招牌：它判的是「两份清单对不对得上」，与包清单对账（check_reconcile.go）同一族，
// 同样不是依赖边。类号与类名由 ADR-0032 第四条定死，不为此另开一类。
//
// 取值从语法树上取（ADR-0037 决定二）：环境名与环节名都是常量，把常量块解成字符串值再比，
// 不靠正则扫源码文本。

// stageTableDir 是随包判据表所在的目录：跟检查目标的根目录走，样本模块可以放一份迷你表。
//
// 表不止一张——改密那一摊自己一份（`parse/tables.go` 的三个路径、`verdict.go` 的两份文件），
// 所以这里读 config 下全部 `.conf`，由表自己说它认哪个环境。
const stageTableDir = "config"

// stageLinePrefix 是报文的首段：`sshfleet.<环境>.<环节>`（ADR-0021 决定 1 条）。
const stageLinePrefix = "sshfleet."

// msgFunc 是拼一行报文的唯一构造点：环境与环节是它的头两个参数。
const msgFunc = "msg"

// CheckStages 判代码里产出的每个环节名在判据表里都有条目。
//
// 「产出一对」在拼报文那一处取：`msg(环境, 环节, 正文)`。两格都从语法树解成字符串值
// （字面量、包级常量、函数里赋过常量的本地变量）；环节名由参数传进来的，到调用点去取。
// 两层以上的转传取不到——今天每一处传参都只有一层，这个深度够用，也免得为一条判据造
// 一张数据流表。
//
// 「哪张表」由表自己说：一张表里只要认了这个环境的任何一条，它就是这个环境的表；于是
// 这个环境的其余环节名也得在它里面。这样既不必在检查器里再抄一份「环境→表」的对应，
// 又能抓住「条目写对了表之外的那一张」——那一条运行时同样认不出来。
func CheckStages(m *Module) []Violation {
	pairs := stagePairs(m)
	if len(pairs) == 0 {
		return nil
	}
	tables, ok := readStageTables(m)
	if !ok {
		return []Violation{violationAt(ClassDeps, stageTableDir,
			"判据表读不到",
			"读不到表就判不出哪个环节名是有条目的：静默放过等于这道检查从来没在跑",
			"把 config 下那几张判据表放回模块根下；它们随包发，路径是模块内相对路径")}
	}
	var out []Violation
	saidEnv := map[string]bool{}
	for _, pr := range pairs {
		token := stageLinePrefix + pr.env + "." + pr.stage
		home := envHome(tables, stageLinePrefix+pr.env+".")
		switch {
		case home == "":
			// 一个环境整张表都不认，是这个环境的一件事：报一次就够，别按环节名刷屏。
			if saidEnv[pr.env] {
				continue
			}
			saidEnv[pr.env] = true
			out = append(out, pr.f.violation(ClassDeps, pr.pos,
				fmt.Sprintf("环境 %s 在判据表里一条都没有", pr.env),
				"环境名也是格式的一部分：一张表都没认这个环境，它下面每个环节名都回落成报错原文",
				fmt.Sprintf("在 config 下的判据表里给这个环境补条目，至少 '%s' 得有一条", token)))
		case strings.Contains(home, "'"+token+"'"):
			continue
		default:
			out = append(out, pr.f.violation(ClassDeps, pr.pos,
				fmt.Sprintf("环节名 %s 在判据表里没有条目", token),
				"判据表只认到环节名：表里没有这一条，这一类失败就回落成报错原文、统计里算未归类——"+
					"使用者看到的是「执行失败」而不是「拒绝网络连接」",
				fmt.Sprintf("在认这个环境的那张判据表里补一条 '%s'，归到该在的分类下；"+
					"确要改名，就同时改表——环节名是契约（ADR-0021）", token)))
		}
	}
	return out
}

// envHome 返回「已经认下这个环境」的那张表的原文；一张都没有返回空串。
//
// 含引号地查在不在：再加冒号或逗号就成了另一套解析，反而跟表的写法各走各的。
func envHome(tables []string, envPrefix string) string {
	for _, t := range tables {
		if strings.Contains(t, "'"+envPrefix) {
			return t
		}
	}
	return ""
}

// readStageTables 读 config 下全部 `.conf` 的原文；一张都读不到返回 false。
func readStageTables(m *Module) ([]string, bool) {
	dir := filepath.Join(m.Dir, filepath.FromSlash(stageTableDir))
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, false
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".conf") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, false
		}
		out = append(out, string(raw))
	}
	return out, len(out) > 0
}

// stagePair 是「某处拼报文时用到的环境与环节」。
type stagePair struct {
	f     *SourceFile
	pos   token.Pos
	env   string
	stage string
}

// stagePairs 收齐模块里所有产出的「环境.环节」。
//
// 只扫拼报文那一处所在的包：环境名与环节名都住在那儿，别的包不拼报文。
func stagePairs(m *Module) []stagePair {
	home := msgHome(m)
	if home == nil {
		return nil
	}
	consts := stringConsts(home)
	c := &stageCollector{
		consts:   consts,
		returned: stagesReturned(home, consts),
		seen:     map[string]bool{},
		deferred: map[string][]stageParam{},
	}
	for _, f := range home.Files {
		if f.IsTest {
			continue
		}
		c.collectDirect(f)
	}
	for _, f := range home.Files {
		if f.IsTest {
			continue
		}
		c.collectThrough(f)
	}
	return c.pairs
}

// msgHome 是定义拼报文那一处的包；没有就返回 nil（样本模块可以不带这条判据）。
func msgHome(m *Module) *Package {
	for _, p := range m.Packages {
		for _, f := range p.Files {
			for _, decl := range f.AST.Decls {
				if fn, ok := decl.(*ast.FuncDecl); ok && fn.Recv == nil && fn.Name.Name == msgFunc {
					return p
				}
			}
		}
	}
	return nil
}

// stageParam 记「这个函数把环节名当参数收进来」，以及它用哪个环境拼。
type stageParam struct {
	at  int
	env string
}

// stageCollector 走一遍拼报文那个包，收出全部「环境.环节」。
type stageCollector struct {
	consts   map[string]string
	returned map[string][]string
	pairs    []stagePair
	seen     map[string]bool
	deferred map[string][]stageParam
}

// collectDirect 收这个函数里直接拼出来的那几对。
func (c *stageCollector) collectDirect(f *SourceFile) {
	c.eachFunc(f, func(fn *ast.FuncDecl) {
		sc := newStageScope(fn, c.consts, c.returned)
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			env, stageExpr, ok := sc.msgCall(n)
			if !ok {
				return true
			}
			if at, isParam := sc.param(stageExpr); isParam {
				c.deferred[fn.Name.Name] = append(c.deferred[fn.Name.Name],
					stageParam{at: at, env: env})
				return true
			}
			for _, stage := range sc.candidates(stageExpr) {
				c.add(stagePair{f: f, pos: stageExpr.Pos(), env: env, stage: stage})
			}
			return true
		})
	})
}

// collectThrough 到调用点去取「当参数传进来的环节名」。
func (c *stageCollector) collectThrough(f *SourceFile) {
	c.eachFunc(f, func(fn *ast.FuncDecl) {
		sc := newStageScope(fn, c.consts, c.returned)
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			id, ok := call.Fun.(*ast.Ident)
			if !ok {
				return true
			}
			for _, d := range c.deferred[id.Name] {
				if d.at >= len(call.Args) {
					continue
				}
				arg := call.Args[d.at]
				for _, stage := range sc.candidates(arg) {
					c.add(stagePair{f: f, pos: arg.Pos(), env: d.env, stage: stage})
				}
			}
			return true
		})
	})
}

// eachFunc 把文件里每个有函数体的函数交给 visit。
func (c *stageCollector) eachFunc(f *SourceFile, visit func(*ast.FuncDecl)) {
	for _, decl := range f.AST.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok && fn.Body != nil {
			visit(fn)
		}
	}
}

// add 记一对；同一对只留第一次见到的那处，报错因此不会按调用点重复。
func (c *stageCollector) add(p stagePair) {
	key := p.env + "." + p.stage
	if c.seen[key] {
		return
	}
	c.seen[key] = true
	c.pairs = append(c.pairs, p)
}

// stageScope 是解一个函数里那两格要用的东西：包级常量、这个函数的参数、它赋过常量的本地变量。
type stageScope struct {
	consts   map[string]string
	returned map[string][]string
	params   map[string]int
	locals   map[string][]string
}

func newStageScope(fn *ast.FuncDecl, consts map[string]string, returned map[string][]string) stageScope {
	sc := stageScope{consts: consts, returned: returned, params: map[string]int{}, locals: map[string][]string{}}
	at := 0
	for _, field := range fn.Type.Params.List {
		for _, name := range field.Names {
			sc.params[name.Name] = at
			at++
		}
	}
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		switch as := n.(type) {
		case *ast.AssignStmt:
			for i, lhs := range as.Lhs {
				if i >= len(as.Rhs) {
					continue
				}
				sc.note(lhs, as.Rhs[i])
			}
		case *ast.ValueSpec:
			for i, name := range as.Names {
				if i >= len(as.Values) {
					continue
				}
				sc.note(name, as.Values[i])
			}
		}
		return true
	})
	return sc
}

// note 记下「这个本地变量被赋过这个常量值」；一个变量赋过两种，两种都记。
func (s *stageScope) note(lhs ast.Expr, rhs ast.Expr) {
	id, ok := lhs.(*ast.Ident)
	if !ok {
		return
	}
	v, ok := s.constValue(rhs)
	if !ok || slices.Contains(s.locals[id.Name], v) {
		return
	}
	s.locals[id.Name] = append(s.locals[id.Name], v)
}

// constValue 把表达式解成包级常量或字面量；解不出来返回 false。
func (s stageScope) constValue(e ast.Expr) (string, bool) {
	if v, ok := stringLiteral(e); ok {
		return v, true
	}
	id, ok := e.(*ast.Ident)
	if !ok {
		return "", false
	}
	v, ok := s.consts[id.Name]
	return v, ok
}

// msgCall 判这个节点是不是拼报文那一处的调用，是就交出环境值与前两个参数里的环节那一格。
func (sc stageScope) msgCall(n ast.Node) (string, ast.Expr, bool) {
	call, ok := n.(*ast.CallExpr)
	if !ok {
		return "", nil, false
	}
	id, ok := call.Fun.(*ast.Ident)
	if !ok || id.Name != msgFunc || len(call.Args) < 2 {
		return "", nil, false
	}
	env, ok := sc.constValue(call.Args[0])
	if !ok {
		return "", nil, false
	}
	return env, call.Args[1], true
}

// param 判这个表达式是不是本函数的参数，是就交出它的位置。
func (sc stageScope) param(e ast.Expr) (int, bool) {
	id, ok := e.(*ast.Ident)
	if !ok {
		return 0, false
	}
	at, ok := sc.params[id.Name]
	return at, ok
}

// candidates 是一个环节名表达式的全部可能值：字面量、常量、赋过常量的本地变量，
// 以及「挨个挑名字」那种辅助函数交回来的那几个值。
func (sc stageScope) candidates(e ast.Expr) []string {
	if v, ok := sc.constValue(e); ok {
		return []string{v}
	}
	switch x := e.(type) {
	case *ast.Ident:
		return sc.locals[x.Name]
	case *ast.CallExpr:
		if id, ok := x.Fun.(*ast.Ident); ok {
			return sc.returned[id.Name]
		}
	}
	return nil
}

// stagesReturned 是包里那些「挑环节名」的辅助函数各自会交出的值。
//
// 拨号与握手都不在拼报文那一处定名，各有一个挑名字的函数（接线错误归哪一格、握手错误
// 归哪一格）；它的每个 return 值都是一处可能产出。漏了它，新加一个名字而忘配表就查不出来
// ——那正是这条判据要报的那件事。
func stagesReturned(p *Package, consts map[string]string) map[string][]string {
	out := map[string][]string{}
	for _, f := range p.Files {
		if f.IsTest {
			continue
		}
		for _, decl := range f.AST.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil || !returnsString(fn) {
				continue
			}
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				ret, ok := n.(*ast.ReturnStmt)
				if !ok {
					return true
				}
				for _, e := range ret.Results {
					if v, ok := (stageScope{consts: consts}).constValue(e); ok {
						out[fn.Name.Name] = append(out[fn.Name.Name], v)
					}
				}
				return true
			})
		}
	}
	return out
}

// returnsString 判一个函数是不是只交字符串：别的返回形状不当环节名的来源。
func returnsString(fn *ast.FuncDecl) bool {
	if fn.Type.Results == nil || len(fn.Type.Results.List) == 0 {
		return false
	}
	for _, field := range fn.Type.Results.List {
		id, ok := field.Type.(*ast.Ident)
		if !ok || id.Name != "string" {
			return false
		}
	}
	return true
}

// stringConsts 是一个包里全部「值是字符串字面量」的包级常量。
func stringConsts(p *Package) map[string]string {
	out := map[string]string{}
	for _, f := range p.Files {
		if f.IsTest {
			continue
		}
		for _, decl := range f.AST.Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok || gen.Tok != token.CONST {
				continue
			}
			for _, spec := range gen.Specs {
				vs, ok := spec.(*ast.ValueSpec)
				if !ok {
					continue
				}
				for i, name := range vs.Names {
					if i >= len(vs.Values) {
						continue
					}
					if v, ok := stringLiteral(vs.Values[i]); ok {
						out[name.Name] = v
					}
				}
			}
		}
	}
	return out
}

// stringLiteral 把一个字符串字面量解成值；反引号与双引号都认。
func stringLiteral(e ast.Expr) (string, bool) {
	lit, ok := e.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return "", false
	}
	v, err := strconv.Unquote(lit.Value)
	if err != nil {
		return "", false
	}
	return v, true
}
