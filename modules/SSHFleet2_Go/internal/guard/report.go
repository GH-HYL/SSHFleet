package guard

import (
	"fmt"
	"go/token"
	"sort"
	"strings"
)

// FenceNote 是每条报错的开头。不说清这是什么红，下一次看到红的第一反应是「测试写错了，改测试」
// ——那正好把闸门拆了（07 号工单答案第十条）。
const FenceNote = "这是结构约束，不是功能测试：红了说明有人绕开了架构边界，不是代码算错了。"

// 七类的名字。台账「规则」一栏、报错里的方括号、map.md 的阶段表都用同一套写法，
// 类号不动（ADR-0032 第四条、ADR-0037；类七见建站 07 规格第六节）。
const (
	ClassTerminal = "类一 写终端"
	ClassDeps     = "类二 依赖方向"
	ClassExit     = "类三 退出动作"
	ClassContract = "类四 契约纯净"
	ClassFailure  = "类五 失败类型的构造点"
	ClassTestSite = "类六 测试站位"
	ClassHandoff  = "类六 交接件更窄"
	ClassMask     = "类七 遮的构造点"
	ClassRedline  = "红线名单与指纹"
)

// allClasses 是跑全的时候的次序，与类号一致。
var allClasses = []string{
	ClassTerminal, ClassDeps, ClassExit, ClassContract,
	ClassFailure, ClassTestSite, ClassHandoff, ClassMask, ClassRedline,
}

// skipTestFiles 说清每一类对 _test.go 的态度。
//
// ADR-0032 第四条只给「写终端」「依赖方向」两条豁免；第五类不扫测试是本检查自己的取舍——
// 它防的是生产路径上把两种失败混用，测试里造一个类型来断言形状不算越界。
// 不在这张表里的类，一律连测试一起看（退出动作、契约纯净、名单与指纹都是这样）。
var skipTestFiles = map[string]bool{
	ClassTerminal: true,
	ClassDeps:     true,
	ClassFailure:  true,
	// 类七也不扫测试：测试里造一句带星号的样本是在断言形状，不是生产路径上自己遮。
	ClassMask: true,
}

// scans 判一条判据要不要看这份文件。
func scans(class string, f *SourceFile) bool {
	return !(f.IsTest && skipTestFiles[class])
}

// Violation 是一条越界。
//
// 四样缺一不可：位置、哪里绕开了（What）、为什么这算绕开（Why）、该走哪一处（Fix）。
// 只写「不许」的报错等于没说——下一个接手的人不知道该往哪儿改。
type Violation struct {
	Class string
	File  string
	Line  int
	What  string
	Why   string
	Fix   string
}

func (v Violation) String() string {
	where := v.File
	if v.Line > 0 {
		where = fmt.Sprintf("%s:%d", v.File, v.Line)
	}
	return fmt.Sprintf("%s [%s] %s。%s。该走：%s", where, v.Class, v.What, v.Why, v.Fix)
}

// violationAt 是整包、整模块一级事实的构造点：这一级没有行号，位置只写文件。
func violationAt(class, file string, what, why, fix string) Violation {
	return Violation{Class: class, File: file, What: what, Why: why, Fix: fix}
}

// violation 是一条违规的唯一构造点：位置由调用处把语法树里的 Pos 交进来。
// 六类都从这里出，报错形态因此只有一处可改。
func (f *SourceFile) violation(class string, pos token.Pos, what, why, fix string) Violation {
	v := violationAt(class, f.Rel, what, why, fix)
	v.Line = f.line(pos)
	return v
}

// Report 把一串违规排成一段话；零违规返回空串。
func Report(vs []Violation) string {
	if len(vs) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(FenceNote)
	b.WriteString("\n")
	for _, v := range sorted(vs) {
		b.WriteString("  ")
		b.WriteString(v.String())
		b.WriteString("\n")
	}
	return b.String()
}

// sorted 按类号、文件、行排序，让同一个问题的报错每次位置一样。
func sorted(vs []Violation) []Violation {
	out := make([]Violation, len(vs))
	copy(out, vs)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Class != out[j].Class {
			return classRank(out[i].Class) < classRank(out[j].Class)
		}
		if out[i].File != out[j].File {
			return out[i].File < out[j].File
		}
		return out[i].Line < out[j].Line
	})
	return out
}

func classRank(class string) int {
	for i, c := range allClasses {
		if c == class {
			return i
		}
	}
	return len(allClasses)
}
