package guard

import "testing"

// 台账空着也要数得出 0 条，不是读不到就静默过。
func TestLoadLedger(t *testing.T) {
	ex, err := LoadLedger("exceptions.md")
	if err != nil {
		t.Fatalf("读台账失败：%v", err)
	}
	if len(ex) != 0 {
		t.Fatalf("台账现在是空的，want 0 条，got %d：%v", len(ex), ex)
	}
}

// 列序以表头为准：md 里调列，读出来的值跟着走，不是按位置硬套。
func TestLoadLedgerHonoursHeader(t *testing.T) {
	ex, err := LoadLedger("testdata/ledger/调序的台账.md")
	if err != nil {
		t.Fatalf("读样本台账失败：%v", err)
	}
	if len(ex) != 1 {
		t.Fatalf("want 1 条，got %d：%v", len(ex), ex)
	}
	got := ex[0]
	want := Exception{
		Rule:  ClassTerminal,
		Where: "internal/parse",
		Why:   "照搬老代码，阶段 04 修",
		Who:   "开发者 2026-11-01",
	}
	if got != want {
		t.Errorf("四列读串了：\nwant %+v\ngot  %+v", want, got)
	}
}

// 命中台账的违规不报；没盖住任何违规的那一行要交出来。
func TestApply(t *testing.T) {
	vs := []Violation{
		{Class: ClassTerminal, File: "internal/parse/a.go", Line: 3},
		{Class: ClassExit, File: "internal/execute/b.go", Line: 9},
	}
	ledger := []Exception{{Rule: ClassTerminal, Where: "internal/parse", Why: "照搬老代码，阶段 04 修", Who: "开发者 2026-11-01"}}

	kept, stale := Apply(vs, ledger)
	if len(kept) != 1 || kept[0].Class != ClassExit {
		t.Fatalf("want 只剩退出动作那一条，got：%v", kept)
	}
	if len(stale) != 0 {
		t.Fatalf("这一行盖住了一条违规，不该报 stale，got：%v", stale)
	}

	unused := []Exception{{Rule: ClassTerminal, Where: "internal/report", Why: "x", Who: "y"}}
	_, stale = Apply(vs, unused)
	if len(stale) != 1 {
		t.Fatalf("没盖住任何违规的那一行要交出来，got：%v", stale)
	}
}
