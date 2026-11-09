package guard

import (
	"strings"
	"testing"
)

// 登记本身：入口文件少一个就要红，报得出缺的是哪一项。
//
// 这条测试自己也跑一遍真工程——它是闸门的第二个触发点：project_test.go 被删掉时，
// 剩这一个仍在跑 CheckAll，于是「登记缺一项：入口」照样会红，不会静默全绿。
func TestCheckRegistry(t *testing.T) {
	m := &Module{
		Target: Target{Dir: "testdata", ModulePath: "sshfleet2"},
		Packages: []*Package{{
			Path:  "sshfleet2/internal/guard",
			Short: "guard",
			Name:  "guard",
			Rule:  &Rule{Package: "guard", Kind: KindCheck},
		}},
	}
	vs := CheckRegistry(m)
	if len(vs) != len(EntryFiles) {
		t.Fatalf("登记了 %d 个入口，应当全报缺失，got %d 条", len(EntryFiles), len(vs))
	}
	if !strings.Contains(vs[0].What, "登记缺一项：入口") {
		t.Fatalf("报错没说清缺的是入口：%s", vs[0].What)
	}

	ledger, err := LoadLedger("exceptions.md")
	if err != nil {
		t.Fatalf("读不到放行台账：%v", err)
	}
	real, err := Load(Target{
		Dir:        "../..",
		ModulePath: "sshfleet2",
		Rules:      Rules(),
		Failures:   DefaultFailures(),
		Redline:    RedLine(),
		Ledger:     ledger,
	})
	if err != nil {
		t.Fatalf("读真模块失败：%v", err)
	}
	if got := CheckAll(real); len(got) != 0 {
		t.Fatalf("真工程应当零违规，got %d 条：\n%s", len(got), Report(got))
	}
}

// 台账四格：缺一格就红，四格齐就闭嘴。
func TestCheckLedgerRequiresFourCells(t *testing.T) {
	vs := CheckLedger([]Exception{{Rule: "类一 写终端", Where: "internal/console"}})
	if len(vs) != 1 {
		t.Fatalf("缺两格的放行应当报一条，got %d 条", len(vs))
	}
	if !strings.Contains(vs[0].What, "为什么放") || !strings.Contains(vs[0].What, "谁批的") {
		t.Fatalf("报错没列出缺哪几格：%s", vs[0].What)
	}
	full := []Exception{{Rule: "类一 写终端", Where: "internal/console", Why: "样本", Who: "开发者"}}
	if got := CheckLedger(full); len(got) != 0 {
		t.Fatalf("四格齐的放行不该报，got %d 条", len(got))
	}
}
