package guard

import (
	"fmt"
	"go/ast"
	"slices"
	"sort"
	"strconv"
	"strings"
)

// RedLineEntry 是红线名单的一行：一条测试，加它的指纹。
//
// 指纹钉两样、不钉字节（ADR-0037 决定五）：测试名，加这条测试**直接**引用到的生产符号。
// gofmt、改注释、重排语句、经 helper 间接引用的都不算改。
// 新增一条不算改；删掉、改名、缩窄引用面算改。
type RedLineEntry struct {
	Test    string
	Symbols []string
}

// redline 是红线名单本体。
//
// 范围是 guard 包里全部 Test 开头的函数：红线区与结构检查同住这一处，所以这一处每加一条
// 测试都要留痕——删掉其中一条，正是「把闸门拆了」最省事的那个动作。
var redline = []RedLineEntry{
	// 01 号工单：地基（扫描器、报错说法、包数）
	{"TestLoadCountsPackages", []string{"Load", "Rules", "Target"}},
	{"TestLoadSkipsDirs", []string{}},
	{"TestCommentAndStringAreNotViolations", []string{"CheckAll", "Report"}},
	{"TestReportKeepsFourParts", []string{"ClassExit", "FenceNote", "Report", "Violation"}},

	// 02 号工单：写终端与退出动作
	{"TestCheckTerminal", []string{"CheckTerminal", "ClassTerminal"}},
	{"TestCheckTerminalExemptsTestFiles", []string{"CheckTerminal"}},
	{"TestCheckExit", []string{"CheckExit", "ClassExit", "Report"}},

	// 03 号工单：依赖方向与包清单对账
	{"TestCheckDeps", []string{"CheckDeps", "Report"}},
	{"TestCheckDepsClean", []string{"CheckDeps", "Report"}},

	// 04 号工单：契约纯净与失败类型的构造点
	{"TestCheckContract", []string{"CheckContract", "ClassContract"}},
	{"TestCheckFailure", []string{"CheckFailure", "Report"}},
	{"TestCheckFailureDetectsMissingType", []string{"CheckFailure", "FailureSite"}},

	// 05 号工单：测试站位
	{"TestCheckTestSite", []string{"CheckTestSite", "Report"}},
	{"TestCheckTestSiteClean", []string{"CheckTestSite", "Report"}},

	// 06 号工单：交接件宽窄
	{"TestCheckHandoff", []string{"CheckHandoff", "PaperPair", "Report"}},
	{"TestCheckHandoffMissingPaper", []string{"CheckHandoff", "PaperPair"}},

	// 07 号工单：名单与台账本身
	{"TestCheckRedline", []string{"CheckRedline", "RedLineEntry", "Report"}},
	{"TestLoadLedger", []string{"LoadLedger"}},
	{"TestLoadLedgerHonoursHeader", []string{"ClassTerminal", "Exception", "LoadLedger", "Rule"}},
	{"TestApply", []string{"Apply", "ClassExit", "ClassTerminal", "Exception", "Rule", "Violation"}},
	// 建站 03：闸门的入口、ask 的那一件事、exitcode 的那一件事
	{"TestRedlineGateRefusesBadCombination", []string{"Kind", "contract.Problem", "contract.ProblemMultiMode", "gate.Run"}},
	{"TestRedlineGatePassesCleanPaper", []string{"gate.Run"}},
	{"TestRedlineAskNonInteractive", []string{"ask.Ask", "ask.Yes", "message.NotWiredLine"}},
	{"TestRedlineExitCodeTable", []string{"Kind", "exitcode.CannotStart", "exitcode.Code", "exitcode.Interrupted", "exitcode.Kind", "exitcode.Refused", "exitcode.Success"}},

	// 真工程整跑
	{"TestProjectIsClean", []string{"CheckAll", "DefaultFailures", "Load", "LoadLedger", "RedLine", "Report", "Rules", "Target"}},

	// 02 阶段：帮助与版本——三种接缝上长出来的头三条（新增不算改，ADR-0037 决定五）
	{"TestRedlineTipRunsHelp", []string{}},
	{"TestRedlineParseSaysOrFail", []string{"parse.Run"}},
	{"TestRedlineMessageRendersHelp", []string{"Kind", "message.EntryOption", "message.HelpEntry", "message.HelpScreen"}},

	// 04 阶段：凭据包的那一件事（新增不算改）
	{"TestRedlineCredentialReportsState", []string{"credential.KeyMissing", "credential.KeyReady", "credential.Secret", "credential.Status"}},
	{"TestRedlineArchiveHandsOutPaths", []string{"archive.Create"}},
	// 04 阶段 09 号单：执行站入口（新增不算改）
	{"TestRedlineExecuteRecordsConnectFact", []string{"contract.CommandTask", "contract.ModeCommand", "contract.Node", "contract.Plan", "contract.Task", "execute.Run"}},
	// 04 阶段 11 号单：第二张纸里没有「成败」（新增不算改）
	{"TestRedlineResultHasNoVerdict", []string{"contract.NodeResult", "contract.ResultPaper"}},
	// 04 阶段 12 号单：那一行真的落到盘上（新增不算改）
	{"TestRedlineExecuteWritesResultLine", []string{"contract.CommandTask", "contract.ModeCommand", "contract.Node", "contract.Plan", "contract.Task", "execute.Run"}},
}

// RedLine 返回名单的一份拷贝。检查读它；改返回值影响不到名单本身。
func RedLine() []RedLineEntry {
	out := make([]RedLineEntry, len(redline))
	for i, e := range redline {
		out[i] = RedLineEntry{Test: e.Test, Symbols: slices.Clone(e.Symbols)}
	}
	return out
}

// CheckRedline 是类七：红线名单与代码对不对得上。
func CheckRedline(m *Module) []Violation {
	var out []Violation

	gp := m.PkgOfKind(KindCheck)
	if gp == nil {
		for _, e := range m.Redline {
			out = append(out, violationAt(ClassRedline, m.ModulePath,
				fmt.Sprintf("名单里有 %s，但工程里没有检查包", e.Test),
				"名单钉的是检查包里的测试；包都没了，这份名单自己就先烂了",
				"把检查包补回来；确要改名单，走议题加 ADR"))
		}
		return out
	}

	listed := map[string]RedLineEntry{}
	for _, e := range m.Redline {
		listed[e.Test] = e
	}

	seen := map[string]bool{}
	for _, f := range gp.Files {
		if !f.IsTest {
			continue
		}
		for _, decl := range f.AST.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv != nil || !strings.HasPrefix(fn.Name.Name, "Test") {
				continue
			}
			name := fn.Name.Name
			seen[name] = true
			got := fingerprint(m, gp, f, fn)
			entry, ok := listed[name]
			if !ok {
				out = append(out, f.violation(ClassRedline, fn.Pos(),
					fmt.Sprintf("红线测试 %s 不在名单里", name),
					"这一处的每一条测试都要留痕：不留名字，删掉它、改小它都没人看得见",
					"把这一行补进 guard/redline.go："+entryLiteral(name, got)))
				continue
			}
			want := slices.Clone(entry.Symbols)
			slices.Sort(want)
			if slices.Equal(want, got) {
				continue
			}
			out = append(out, f.violation(ClassRedline, fn.Pos(),
				fmt.Sprintf("红线测试 %s 的指纹对不上：名单里 %v，现在是 %v", name, want, got),
				"指纹钉住这条测试直接引用到的生产符号：符号一变，说明它测的东西被改窄、改宽或挪走了",
				"确认是有意的改动之后改名单："+entryLiteral(name, got)+"——改名单是红线，走议题加 ADR"))
		}
	}

	for _, e := range m.Redline {
		if seen[e.Test] {
			continue
		}
		out = append(out, violationAt(ClassRedline, gp.Path,
			fmt.Sprintf("名单里的 %s 在代码里已经没有了", e.Test),
			"删一条红线测试就算改红线：名单还留着，说明它被删掉却没走议题加 ADR",
			"把测试补回来；确要删，先开议题写 ADR，再动名单"))
	}
	return out
}

// fingerprint 算一条测试的指纹：它直接引用到的生产符号，去重升序。
//
// 两样都算：同包里被测试体点名的顶层名字（入口函数、类型、包级变量），
// 以及它对工程内包的点名引用（pkg.Sym）。经 helper 间接引用的不算——只看这一个函数体。
func fingerprint(m *Module, p *Package, f *SourceFile, fn *ast.FuncDecl) []string {
	prod := productionNames(p)
	set := map[string]bool{}
	if fn.Body == nil {
		return nil
	}
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.Ident:
			if prod[x.Name] {
				set[x.Name] = true
			}
		case *ast.SelectorExpr:
			id, ok := x.X.(*ast.Ident)
			if !ok {
				return true
			}
			short, ok := m.Internal(f.ImportPath(id.Name))
			if !ok {
				return true
			}
			set[short+"."+x.Sel.Name] = true
		}
		return true
	})
	out := make([]string, 0, len(set))
	for name := range set {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// productionNames 是一个包里非测试文件的顶层名字。
func productionNames(p *Package) map[string]bool {
	out := map[string]bool{}
	for _, f := range p.Files {
		if f.IsTest {
			continue
		}
		for _, decl := range f.AST.Decls {
			switch d := decl.(type) {
			case *ast.FuncDecl:
				if d.Recv == nil {
					out[d.Name.Name] = true
				}
			case *ast.GenDecl:
				for _, spec := range d.Specs {
					switch s := spec.(type) {
					case *ast.TypeSpec:
						out[s.Name.Name] = true
					case *ast.ValueSpec:
						for _, name := range s.Names {
							out[name.Name] = true
						}
					}
				}
			}
		}
	}
	return out
}

func entryLiteral(name string, symbols []string) string {
	quoted := make([]string, 0, len(symbols))
	for _, s := range symbols {
		quoted = append(quoted, strconv.Quote(s))
	}
	return fmt.Sprintf("{%s, []string{%s}}", strconv.Quote(name), strings.Join(quoted, ", "))
}
