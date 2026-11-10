package guard

import (
	"path/filepath"
	"strings"
	"testing"
)

// 类四的纸面白名单：样本里 Node.Note 是一格裸 string，名单上没有它，要报出来。
//
// 白名单按模块注入：真工程不传，检查器用 paper.go 那张自带的；样本传一张小的，
// 顺带证明「名单里多一行、契约里已经没有」那个方向不误报（Node.IP 在，Node.Note 不在）。
func TestCheckContractPaper(t *testing.T) {
	m, err := Load(Target{
		Dir:        filepath.Join("testdata", "paper"),
		ModulePath: "fixture",
		Rules:      []Rule{{Package: "contract", Kind: KindContract}},
		Paper:      []PaperField{{"Node", "IP", PaperFact}},
	})
	if err != nil {
		t.Fatalf("读样本 paper 失败：%v", err)
	}

	got := inClass(CheckContract(m), ClassContract)
	if len(got) != 1 {
		t.Fatalf("纸面白名单 want 1 条，got %d 条：%v", len(got), got)
	}
	if !strings.Contains(got[0].String(), "Node.Note") {
		t.Errorf("报的应当是 Node.Note 这一格，got：%s", got[0].String())
	}
}
