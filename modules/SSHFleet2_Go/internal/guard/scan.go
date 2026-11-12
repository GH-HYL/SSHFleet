package guard

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	pathpkg "path"
	"path/filepath"
	"slices"
	"strings"
)

// Target 是一次检查的对象：一个模块根，加上判它要用的那几样数据。
//
// 真工程由 project_test.go 组一个；样本模块由各 Check* 的单测组一个。
// 除 Dir 与 ModulePath 外，其余字段都可以是零值——零值就是那一类还没东西可判。
type Target struct {
	// Dir 是模块根的绝对路径；走目录的范围与 go list ./... 同口径。
	Dir string
	// ModulePath 是模块名，如 sshfleet2。塔尖的包路径就是它本身。
	ModulePath string
	// Rules 是包清单规则表，十八行。
	Rules []Rule
	// Failures 是类五要判的两处：哪种失败许在哪个包造。
	Failures []FailureSite
	// Pairs 是类六之二要判的纸：同一口径上相邻两棒的一对类型。
	Pairs []PaperPair
	// Redline 是红线名单与指纹；类七拿它对照代码。
	Redline []RedLineEntry
	// Paper 是纸面白名单；nil 表示用检查器自带的那张（只对 ssHfleet2 这个模块生效）。
	// 样本模块传一张小的，量自己的契约。
	Paper []PaperField
	// Ledger 是放行台账；命中的违规不报。
	Ledger []Exception
}

// Module 是一个模块读进来的样子。
type Module struct {
	Target
	fset     *token.FileSet
	Packages []*Package
	// AsmFiles 是模块里全部 `.s` 汇编文件（模块内相对路径）。
	// 它们不参与 import 解析，却能 CALL 任意符号——所以得单独记一笔（ADR-0049）。
	AsmFiles []string
}

// asmExt 是 Go 汇编文件的后缀：它出现在工程里，就说明有一条不经 import 的调用边。
const asmExt = ".s"

// Package 是一个包：规则表里的一行，加上它的文件。
type Package struct {
	// Path 是模块内完整路径，如 sshfleet2/internal/parse；塔尖是模块名本身。
	Path string
	// Short 是规则表里那一格的名字：站与横切写短名，塔尖写模块名。
	Short string
	// Name 是包名，如 parse、main。
	Name string
	Rule *Rule
	// Rule 为 nil 表示规则表里没有这个包——类二（依赖方向）会报「多一个包」。
	Files []*SourceFile
}

// SourceFile 是一个源文件读进来的样子。
type SourceFile struct {
	Pkg *Package
	// Rel 是模块内相对路径，斜杠分隔，与台账里写的「位置」同形。
	Rel string
	// IsTest 按文件名后缀划，不按目录划（ADR-0032 第四条）。
	IsTest bool
	// IsRedline 标「站在接缝上的红线测试」：文件名以 redline_ 开头且以 _test.go 结尾。
	IsRedline bool
	AST       *ast.File
	// Imports 是这份文件 import 的表，按文件里的写法解得动的名字索引。
	Imports map[string]string
	// SideImports 是 `_` 与 `.` 两种写法的 import 路径：名字解不出来，边照样在（ADR-0048）。
	SideImports []string
	// importPos 记每条 import 在文件里的位置，报错要指到那一行。
	importPos map[string]token.Pos
	fset      *token.FileSet
}

// Import 是文件里的一条 import，判几类都要用。
type Import struct {
	// Name 是这份文件里引用它时写的名字：有别名用别名，没有就用路径最后一段。
	Name string
	Path string
}

// Load 把模块根读成「包 → 文件 → 语法树」。
//
// 判据按语法树数，不按字搜（ADR-0037 决定二）：注释里写的、字符串里带的都不算越界。
func Load(t Target) (*Module, error) {
	root, err := filepath.Abs(t.Dir)
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(root); err != nil {
		return nil, fmt.Errorf("检查器找不到模块根 %s：%w", root, err)
	}

	m := &Module{Target: t, fset: token.NewFileSet()}

	walkErr := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			if strings.HasSuffix(d.Name(), asmExt) {
				if rel, rerr := filepath.Rel(root, p); rerr == nil {
					m.AsmFiles = append(m.AsmFiles, filepath.ToSlash(rel))
				}
			}
			return nil
		}
		if p != root && skipDir(d.Name()) {
			return fs.SkipDir
		}
		pkg, err := m.readDir(root, p)
		if err != nil || pkg == nil {
			return err
		}
		m.Packages = append(m.Packages, pkg)
		return nil
	})
	if walkErr != nil {
		return nil, walkErr
	}

	slices.SortFunc(m.Packages, func(a, b *Package) int { return strings.Compare(a.Path, b.Path) })
	for _, p := range m.Packages {
		for _, f := range p.Files {
			f.Pkg = p
		}
	}
	return m, nil
}

// skipDir 是 go list ./... 不看的目录：样本、vendor、以点或下划线开头。
func skipDir(name string) bool {
	return name == "testdata" || name == "vendor" ||
		strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_")
}

func (m *Module) readDir(root, dir string) (*Package, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	rel, err := filepath.Rel(root, dir)
	if err != nil {
		return nil, err
	}
	rel = filepath.ToSlash(rel)

	var pkg *Package
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") {
			continue
		}
		file, err := m.parseFile(root, dir, e.Name())
		if err != nil {
			return nil, err
		}
		if pkg == nil {
			pkg = m.newPackage(rel, file.AST.Name.Name)
		}
		file.Pkg = pkg
		pkg.Files = append(pkg.Files, file)
	}
	return pkg, nil
}

func (m *Module) newPackage(rel, name string) *Package {
	short := m.ModulePath
	path := m.ModulePath
	if rel != "." {
		short = pathpkg.Base(rel)
		path = m.ModulePath + "/" + rel
	}
	p := &Package{Path: path, Short: short, Name: name}
	for i := range m.Rules {
		if m.Rules[i].Package == short {
			p.Rule = &m.Rules[i]
			break
		}
	}
	return p
}

func (m *Module) parseFile(root, dir, name string) (*SourceFile, error) {
	full := filepath.Join(dir, name)
	// 注释要读：`//go:linkname` 是编译指令，不住语法树、只住在注释里（ADR-0049）。
	// 别的判据仍然只看语法树——「注释里提到 os.Exit」不算越界，这条不变（ADR-0037 决定二）。
	astFile, err := parser.ParseFile(m.fset, full, nil, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("检查器读不动 %s：%w", full, err)
	}
	rel, err := filepath.Rel(root, full)
	if err != nil {
		return nil, err
	}
	file := &SourceFile{
		Rel:       filepath.ToSlash(rel),
		IsTest:    strings.HasSuffix(name, "_test.go"),
		IsRedline: strings.HasSuffix(name, "_test.go") && strings.HasPrefix(name, "redline_"),
		AST:       astFile,
		Imports:   map[string]string{},
		importPos: map[string]token.Pos{},
		fset:      m.fset,
	}
	for _, spec := range astFile.Imports {
		path := strings.Trim(spec.Path.Value, `"`)
		local := pathpkg.Base(path)
		if spec.Name != nil {
			local = spec.Name.Name
		}
		file.importPos[path] = spec.Pos()
		if local == "_" || local == "." {
			// 名字解不出来，但这条边是真的：空白 import 会跑 init、点 import 把符号倒进
			// 本文件的作用域——两种都够拿它反向依赖了（ADR-0048）。
			file.SideImports = append(file.SideImports, path)
			continue
		}
		file.Imports[local] = path
	}
	return file, nil
}

// line 把语法树里的位置换成行号。
func (f *SourceFile) line(pos token.Pos) int {
	if !pos.IsValid() {
		return 0
	}
	return f.fset.Position(pos).Line
}

// ImportPath 返回这份文件里用 name 引用的那个 import 的路径；没有就返回空。
func (f *SourceFile) ImportPath(name string) string {
	return f.Imports[name]
}

// Internal 把一条 import 路径翻成规则表里的短名；不是本模块的返回 false。
func (m *Module) Internal(path string) (string, bool) {
	switch {
	case path == m.ModulePath:
		return m.ModulePath, true
	case strings.HasPrefix(path, m.ModulePath+"/"):
		return pathpkg.Base(path), true
	default:
		return "", false
	}
}

// Pkg 按规则表里的短名找包；没有就返回 nil。
func (m *Module) Pkg(short string) *Package {
	for _, p := range m.Packages {
		if p.Short == short {
			return p
		}
	}
	return nil
}

// IsStdlib 判一条 import 是不是标准库：首段不带点就是。
func IsStdlib(path string) bool {
	first, _, _ := strings.Cut(path, "/")
	return !strings.Contains(first, ".")
}

// PkgPath 把一个短名换算成模块内完整路径。
//
// 规则表那一行写短名，十六个包都在 internal/ 下一级；塔尖那一行写模块名，换算回来就是模块名本身。
// 包清单对账（类二）会保证表与目录一一对上，所以这条换算与磁盘上的形状不会各走各的。
func (m *Module) PkgPath(short string) string {
	if short == m.ModulePath {
		return m.ModulePath
	}
	return m.ModulePath + "/internal/" + short
}

// PkgOfKind 按规则表里的类找包；同一类有多个时返回第一个。
//
// 契约包与检查包各只有一行，所以「按类找」在它们身上没有歧义。
func (m *Module) PkgOfKind(k Kind) *Package {
	for _, p := range m.Packages {
		if p.Rule != nil && p.Rule.Kind == k {
			return p
		}
	}
	return nil
}

// IsContract 判一个短名是不是契约包——问规则表那一行的类，不问名字。
func (m *Module) IsContract(short string) bool {
	p := m.Pkg(short)
	return p != nil && p.Rule != nil && p.Rule.Kind == KindContract
}

// TypeSpec 找一个包里某个类型的声明。找类型的判据都走这一处。
func TypeSpec(p *Package, name string) (*SourceFile, *ast.TypeSpec, bool) {
	for _, f := range p.Files {
		for _, decl := range f.AST.Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok {
				continue
			}
			for _, spec := range gen.Specs {
				ts, ok := spec.(*ast.TypeSpec)
				if ok && ts.Name.Name == name {
					return f, ts, true
				}
			}
		}
	}
	return nil, nil, false
}
