package guard

import "testing"

// 真工程：二十个包、零违规。
func TestProjectIsClean(t *testing.T) {
	ledger, err := LoadLedger("exceptions.md")
	if err != nil {
		t.Fatalf("读不到放行台账：%v", err)
	}

	m, err := Load(Target{
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

	if n := len(Rules()); n != 20 {
		t.Fatalf("规则表 want 20 行，got %d", n)
	}

	vs := CheckAll(m)
	t.Logf("包清单 %d 个；当前放行 %d 条；红线 %d 条",
		len(m.Packages), len(ledger), len(RedLine()))
	if len(vs) != 0 {
		t.Fatalf("真工程应当零违规，got %d 条：\n%s", len(vs), Report(vs))
	}
}
