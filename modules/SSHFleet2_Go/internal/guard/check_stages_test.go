package guard

import (
	"path/filepath"
	"strings"
	"testing"
)

// 样本模块：一个只在判据表齐全时绿，一个缺条目、缺整张表。

// loadStageFixture 读一个样本模块；这条判据不看包清单规则表，所以不带规则。
func loadStageFixture(t *testing.T, name string) *Module {
	t.Helper()
	m, err := Load(Target{Dir: filepath.Join("testdata", name), ModulePath: "fixture"})
	if err != nil {
		t.Fatalf("读样本 %s 失败：%v", name, err)
	}
	return m
}

// 环节名三种来法（直传常量、经一层传参、先赋本地变量）都收得着，条目都在就不红。
func TestCheckStagesListsEveryProducedName(t *testing.T) {
	m := loadStageFixture(t, "stages")

	if vs := CheckStages(m); len(vs) != 0 {
		t.Fatalf("样本里每个环节名都有条目，应当零违规，got %d 条：\n%s", len(vs), Report(vs))
	}

	// 收得全不全单看「零违规」判不出来：漏收一对也还是零违规。这里点名那几对。
	want := []string{
		"run.timeout",      // 直传常量
		"run.rejected",     // 经一层传参，只有调用点看得见
		"passwd.give_up",   // 先赋本地变量
		"passwd.no_prompt", // 同一个变量的另一个取值
	}
	got := map[string]bool{}
	for _, p := range stagePairs(m) {
		got[p.env+"."+p.stage] = true
	}
	for _, w := range want {
		if !got[w] {
			t.Errorf("没收到 %s 这一对：取值那条路上有漏", w)
		}
	}
}

// 判据表少一条、或整张表不在，都红：前者是漏配，后者是这个环境的环节名一条都认不出来。
func TestCheckStagesFailsOnMissingEntry(t *testing.T) {
	m := loadStageFixture(t, "stages_gap")

	vs := inClass(CheckStages(m), ClassDeps)
	if len(vs) != 2 {
		t.Fatalf("want 2 条（一个环节名漏配、一个环境整张表都不认——整张表按环境报一次），got %d 条：%v", len(vs), vs)
	}

	var sawStage, sawEnv bool
	for _, v := range vs {
		if v.File != "execute/message.go" || v.Line == 0 {
			t.Errorf("位置该指到拼报文的那份源码：%+v", v)
		}
		if strings.Contains(v.What, "sshfleet.run.rejected") {
			sawStage = true
		}
		if strings.Contains(v.What, "环境 passwd") {
			sawEnv = true
		}
		if v.Why == "" || v.Fix == "" {
			t.Errorf("四样没写全：%+v", v)
		}
	}
	if !sawStage || !sawEnv {
		t.Errorf("两种红都该报：缺条目 %v、缺整张表 %v", sawStage, sawEnv)
	}
}
