package criterion

import "testing"

// 读法：BOM、`#` 注释、空行、引号、行号；判据：恰好一条、首列是 IPv4。
//
// 那四条畸形输入来自本议题 `audit.md` §三·1——它们正是两处口径曾经结论相反的地方，
// 这里钉死「读不出」。

func TestReadCSVRows(t *testing.T) {
	rows, lines, err := ReadCSVRows("\ufeff# 注释\n\n10.0.0.1,22,root\n10.0.0.2,\"a,b\",pw\n")
	if err != nil {
		t.Fatalf("不该读失败：%v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("应得 2 条记录，实得 %d：%v", len(rows), rows)
	}
	if lines[0] != 3 || lines[1] != 4 {
		t.Fatalf("行号该是 3、4（空行与注释行占号），实得 %v", lines)
	}
	if rows[1][1] != "a,b" {
		t.Fatalf("引号里的逗号该算同一段，实得 %q", rows[1][1])
	}
}

func TestReadCSVRowsRejectsMalformedQuotes(t *testing.T) {
	for _, bad := range []string{
		`10.0.0.1,22,root,p"w`,
		`10.0.0.1,22,root,"p"w`,
		`10.0.0.1,22,root,pw"`,
		`10.0.0.1,22,ro"ot,pw`,
	} {
		if _, _, err := ReadCSVRows(bad); err == nil {
			t.Errorf("%q 该读不出（引号不成对）", bad)
		}
	}
}

func TestLooksLikeInlineNodeRecord(t *testing.T) {
	for _, ok := range []string{
		"10.0.0.1",
		"10.0.0.1,22,root,pw",
		`"10.0.0.1",22,root`,
	} {
		if !LooksLikeInlineNodeRecord(ok) {
			t.Errorf("%q 该算一条内联节点记录", ok)
		}
	}
	for _, bad := range []string{
		"",                         // 空
		"10.0.0.1,22\n10.0.0.2,22", // 不止一条
		"010.0.0.1,22,root",        // 首列不是合法 IPv4
		"10.0.0.1,22,root,p\"w",    // 读不成一条
		"  ",                       // 只有空白
	} {
		if LooksLikeInlineNodeRecord(bad) {
			t.Errorf("%q 不该算一条内联节点记录", bad)
		}
	}
}
