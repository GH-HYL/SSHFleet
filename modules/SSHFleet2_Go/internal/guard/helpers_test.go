package guard

import (
	"path/filepath"
	"strings"
	"testing"
)

// loadFixture 读一个假模块；模块名统一是 fixture，样本里的 import 都按这个写。
func loadFixture(t *testing.T, name string) *Module {
	t.Helper()
	m, err := Load(Target{
		Dir:        filepath.Join("testdata", name),
		ModulePath: "fixture",
		Rules:      fixtureRules(name),
		Failures:   DefaultFailures(),
	})
	if err != nil {
		t.Fatalf("读样本 %s 失败：%v", name, err)
	}
	return m
}

// fixtureRules 是假模块的规则表：与样本目录里真实存在的包一一对上。
//
// 每个样本各写一张，是因为规则表本来就是这个模块的尺子——样本要能自己证明「尺子量得出偏差」。
func fixtureRules(name string) []Rule {
	switch name {
	case "violations":
		// 表里多一行 log（目录里没有），目录里多一个 extra（表里没有）。
		return []Rule{
			{"parse", KindStation, []string{"contract", "console", "message"}},
			{"gate", KindStation, []string{"contract", "console", "message"}},
			{"contract", KindContract, nil},
			{"console", KindCrosscut, nil},
			{"message", KindCrosscut, []string{"contract"}},
			{"ask", KindCrosscut, []string{"contract"}},
			{"log", KindCrosscut, []string{"contract"}},
			{"fixture", KindTip, []string{"parse", "gate"}},
		}
	case "testsite":
		return []Rule{
			{"parse", KindStation, []string{"contract"}},
			{"gate", KindStation, []string{"contract"}},
			{"contract", KindContract, nil},
			{"guard", KindCheck, nil},
			{"fixture", KindTip, nil},
		}
	case "writer":
		// 类一第三条判据的样本：解析站与执行站各取一次 console 的 writer。
		return []Rule{
			{"parse", KindStation, []string{"contract", "console"}},
			{"execute", KindStation, []string{"contract", "console"}},
			{"contract", KindContract, nil},
			{"console", KindCrosscut, nil},
			{"guard", KindCheck, nil},
			{"fixture", KindTip, nil},
		}
	case "handoff":
		return []Rule{
			{"contract", KindContract, nil},
		}
	case "clock":
		// 类二第三条判据的样本：许读的三个家各一次（不红），解析站一次（红）。
		return []Rule{
			{"parse", KindStation, nil},
			{"execute", KindStation, nil},
			{"log", KindCrosscut, nil},
			{"fixture", KindTip, []string{"parse", "execute", "log"}},
		}
	default: // clean
		return []Rule{
			{"parse", KindStation, []string{"contract", "console", "message"}},
			{"gate", KindStation, []string{"contract"}},
			{"contract", KindContract, nil},
			{"console", KindCrosscut, nil},
			{"message", KindCrosscut, []string{"contract"}},
			{"ask", KindCrosscut, []string{"contract", "console", "message"}},
			{"fixture", KindTip, []string{"parse", "gate", "console"}},
		}
	}
}

// inClass 挑出某一类的违规。
func inClass(vs []Violation, class string) []Violation {
	var out []Violation
	for _, v := range vs {
		if v.Class == class {
			out = append(out, v)
		}
	}
	return out
}

// wantOne 断言这一类里有且只有一条，且位置与话术里该有的词都在。
func wantOne(t *testing.T, vs []Violation, class, file, snippet string) {
	t.Helper()
	got := inClass(vs, class)
	if len(got) != 1 {
		t.Fatalf("%s：want 1 条，got %d 条：%v", class, len(got), got)
	}
	v := got[0]
	if v.File != file {
		t.Errorf("%s：位置 want %s，got %s", class, file, v.File)
	}
	text := v.String()
	if !strings.Contains(text, snippet) {
		t.Errorf("%s：话术里没出现 %q：%s", class, snippet, text)
	}
	if v.What == "" || v.Why == "" || v.Fix == "" || v.Line == 0 {
		t.Errorf("%s：四样没写全（行号／哪里绕开／为什么／该走哪一处）：%+v", class, v)
	}
}
