package main

// 开不出归档执行日志时，塔尖要停在启动段（审计 09-3 那一条的判据）。
//
// **怎么到达那一支**：它只在「有留痕要写进执行日志」时才走——`--yes` 跳过确认本身不算
// 留痕，危险命令的提醒档放行才算；而只有执行日志打不开才会触发。归档目录名是
// 「秒级时间戳_模式_备注」拼的，所以先把前后一段秒数的目录全建好，每个里面放一个**同名
// 目录**顶住 `SSHFleetExec.log`：`os.OpenFile` 打不开一个目录。
//
// **窗口取宽（前 5 秒到后 30 秒），而且过期了会重布一次**：布场到进程真的走到建目录那一步
// 之间，隔着进程启动与几份判据表的读取；机器忙的时候这段能拖上十几秒，窗口窄了就落在外头，
// 现象是「进程正常跑完、退 0」——2026-11-18 在全量里撞到过一次。落在外头是布场没踩到，
// 不是被测行为，所以重布；两次都过期就明说机器太慢，不再装作行为不对。
//
// **接缝用现成的进程出入口**，不为这一支给塔尖加口子（ADR-0055）：加一个「可替换的退出
// 值」正是类三禁的那种写法（`os.Exit` 存成函数值再调，检查器看不见），而给生产代码加
// 只给测试用的钩子，比这一支没有测试更糟。

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// 归档目录名的两段：模式与备注。与 `archive.Create` 的拼法同源，改那边这里要跟着改。
const (
	execLogProbeMode   = "command"
	execLogProbeRemark = "probe"
)

func TestExecLogCannotOpenStopsAtStartup(t *testing.T) {
	bin := orderBinary(t)

	module, err := os.Getwd()
	if err != nil {
		t.Fatalf("取当前目录失败：%v", err)
	}

	// 两轮：第一轮落在窗口外（布场没踩到）就重布一次。
	for attempt := 1; attempt <= 2; attempt++ {
		dir := stageOrderDir(t, module, orderRow{})
		blocked := blockExecLogDirs(t, dir)

		code, out := runExecLogProbe(t, bin, dir)
		if code == 0 {
			// 退 0 有两种可能：行为不对，或者进程没落在布场的那几秒里。后者能查出来：
			// history 下冒出没顶住的目录，就说明它自己建了一个新的。
			if stray := strayArchiveDirs(t, dir, blocked); len(stray) > 0 {
				t.Logf("布场窗口过期：进程落在 %v，重布一次", stray)
				continue
			}
			t.Fatalf("执行日志开不出来，不该退 0：\n%s", out)
		}
		// 那一屏要说清是执行日志开不出来，而不是含混的「出错了」。
		for _, want := range []string{"[ERROR]", "建执行期日志失败"} {
			if !strings.Contains(out, want) {
				t.Fatalf("那一屏缺「%s」：\n%s", want, out)
			}
		}
		// 停在启动段：本次归档目录里没有报告、没有终端记录——一个节点都没连。
		for _, at := range blocked {
			archiveDir := filepath.Dir(at)
			for _, name := range []string{"report.txt", "terminal.txt"} {
				if _, err := os.Stat(filepath.Join(archiveDir, name)); err == nil {
					t.Fatalf("停在启动段，不该产出 %s：%s", name, archiveDir)
				}
			}
		}
		return
	}
	t.Fatalf("两次布场都过期：这台机器从起进程到建归档目录要超过 30 秒，先查负载")
}

// blockExecLogDirs 布场：把前 5 秒到后 30 秒的归档目录全建好，每个用**同名目录**顶住
// 执行日志那个文件名（`os.OpenFile` 打不开一个目录）。交回那些被顶住的路径。
//
// 窗口往宽了取，是因为布场到进程建目录之间隔着进程启动与几份判据表的读取：机器忙时
// 这段能拖上十几秒，窗口窄了就落在外头。
func blockExecLogDirs(t *testing.T, dir string) []string {
	t.Helper()

	hist := filepath.Join(dir, "history")
	if err := os.MkdirAll(hist, 0o755); err != nil {
		t.Fatalf("建历史目录失败：%v", err)
	}
	blocked := make([]string, 0, 36)
	for i := -5; i <= 30; i++ {
		name := time.Now().Add(time.Duration(i)*time.Second).Format("2006-01-02_15-04-05") +
			"_" + execLogProbeMode + "_" + execLogProbeRemark
		at := filepath.Join(hist, name, "SSHFleetExec.log")
		if err := os.MkdirAll(at, 0o755); err != nil {
			t.Fatalf("建顶住的目录失败：%v", err)
		}
		blocked = append(blocked, at)
	}
	return blocked
}

// runExecLogProbe 起进程跑那一趟：提醒档的危险命令 + `--yes`，放行之后留痕，才走到
// 「写进执行日志」那一步。交回退出码与整屏输出。
func runExecLogProbe(t *testing.T, bin, dir string) (int, string) {
	t.Helper()

	cmd := exec.Command(bin, "-c", "chmod 777 file.sh", "-f", "ok.csv", "--yes", "-r", execLogProbeRemark)
	cmd.Dir = dir
	var buf strings.Builder
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	runErr := cmd.Run()
	out := buf.String()

	if exit, ok := runErr.(*exec.ExitError); ok {
		return exit.ExitCode(), out
	}
	if runErr != nil {
		t.Fatalf("起二进制失败：%v", runErr)
	}
	return 0, out
}

// strayArchiveDirs 找出本次运行自己建的归档目录：布场顶住的那些之外还算存在的。
//
// 有就说明进程没落在窗口里——那不是被测的行为，是布场过期。
func strayArchiveDirs(t *testing.T, dir string, blocked []string) []string {
	t.Helper()

	ours := map[string]bool{}
	for _, at := range blocked {
		ours[filepath.Dir(at)] = true
	}
	entries, err := os.ReadDir(filepath.Join(dir, "history"))
	if err != nil {
		t.Fatalf("读历史目录失败：%v", err)
	}
	var stray []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		at := filepath.Join(dir, "history", e.Name())
		if !ours[at] {
			stray = append(stray, e.Name())
		}
	}
	return stray
}

// 上面那条判的是「撞上就打不开时会停」；这一条判「撞上之后不会改成继续跑」。
//
// 为什么要多看一条：两处开执行期日志现在同一个口径（塔尖与执行站），屏上的话一字不差，
// 而 `log.Logger` 的方法对 nil 是空动作——把塔尖那一支改成「记一句就往下走」，进程层
// 看不出任何区别（实测：把 `if lerr != nil` 改成 `if lerr != nil && false`，上面那条
// 仍绿）。所以这一支的形状只能用源码形状判：`log.InitExec` 的失败分支必须收在 `fatal`。
func TestExecLogInitFailureEndsAtFatal(t *testing.T) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "main.go", nil, 0)
	if err != nil {
		t.Fatalf("读 main.go 失败：%v", err)
	}

	hits := 0
	ast.Inspect(f, func(n ast.Node) bool {
		block, ok := n.(*ast.BlockStmt)
		if !ok {
			return true
		}
		for i, stmt := range block.List {
			as, ok := stmt.(*ast.AssignStmt)
			if !ok {
				continue
			}
			lhs, ok := assignedFromInitExec(as)
			if !ok {
				continue
			}
			hits++
			// 紧跟着的那一句必须是 `if <那个变量> != nil { … fatal(…) }`。
			next, ok := stmtAt(block, i+1)
			if !ok {
				t.Fatalf("main.go:%d：开完执行期日志没有紧跟一句判错", fset.Position(as.Pos()).Line)
			}
			ifs, ok := next.(*ast.IfStmt)
			if !ok || ifs.Init != nil {
				t.Fatalf("main.go:%d：开完执行期日志没有紧跟一句判错", fset.Position(as.Pos()).Line)
			}
			if !isNotNilOf(ifs.Cond, lhs) {
				t.Fatalf("main.go:%d：判错的条件被改过——要裸的「%s != nil」，不许 `&& 别的`",
					fset.Position(ifs.Pos()).Line, lhs)
			}
			if !callsFatal(ifs.Body) {
				t.Fatalf("main.go:%d：开执行期日志失败没有收在 fatal——这一支就是 09-3 定的「开不出来就停」",
					fset.Position(ifs.Pos()).Line)
			}
		}
		return true
	})
	if hits != 1 {
		t.Fatalf("main.go 里「开执行期日志」该恰好一处，实际 %d 处", hits)
	}
}

// assignedFromInitExec 报这一句是不是 `…, name := log.InitExec(...)`，是的话交回 name。
func assignedFromInitExec(as *ast.AssignStmt) (string, bool) {
	if len(as.Rhs) != 1 {
		return "", false
	}
	call, ok := as.Rhs[0].(*ast.CallExpr)
	if !ok {
		return "", false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "InitExec" {
		return "", false
	}
	// 交回最后那一格：`execLog, lerr := ...` 里判的是 `lerr`，不是那个句柄。
	for i := len(as.Lhs) - 1; i >= 0; i-- {
		if id, ok := as.Lhs[i].(*ast.Ident); ok {
			return id.Name, true
		}
	}
	return "", false
}

// stmtAt 取块里第 i 句；越界算没有。
func stmtAt(block *ast.BlockStmt, i int) (ast.Stmt, bool) {
	if i < 0 || i >= len(block.List) {
		return nil, false
	}
	return block.List[i], true
}

// isNotNilOf 报 cond 是不是裸的「name != nil」。
func isNotNilOf(cond ast.Expr, name string) bool {
	bin, ok := cond.(*ast.BinaryExpr)
	if !ok || bin.Op != token.NEQ {
		return false
	}
	left, ok := bin.X.(*ast.Ident)
	if !ok || left.Name != name {
		return false
	}
	right, ok := bin.Y.(*ast.Ident)
	return ok && right.Name == "nil"
}

// callsFatal 报这一块里有没有调 `fatal(...)`。
func callsFatal(block *ast.BlockStmt) bool {
	found := false
	ast.Inspect(block, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if id, ok := call.Fun.(*ast.Ident); ok && id.Name == "fatal" {
			found = true
			return false
		}
		return true
	})
	return found
}
