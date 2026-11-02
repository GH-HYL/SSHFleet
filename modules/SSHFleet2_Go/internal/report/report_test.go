package report

// 报告站那一个入口：写出来的是什么、解不开时怎么收场。
//
// 逐字版式在 `message` 的 stats_test.go 里判（那边的期望值是从老版真跑产物录的）；
// 这里判的是本站自己的事——落盘、遮秘密、拿不到运行上下文时不顶包。

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sshfleet2/internal/contract"
)

// ledgerForRun 造一张与 runMetaVector 那段密文配套的第三张纸：那份密文里装的是
// `sshfleet2 -f nodes.csv -c uptime`，一次成功、1 台。
func ledgerForRun(t *testing.T) contract.Ledger {
	t.Helper()
	sealed, err := base64.StdEncoding.DecodeString(runMetaVector)
	if err != nil {
		t.Fatalf("样本本身解不开：%v", err)
	}
	return contract.Ledger{
		Total: 1, Done: 1, Verify: "通过",
		SuccessCounts: 1, SuccessCategory: "执行成功",
		CategoryIPs: []contract.CategoryIPs{{Category: "执行成功", IPs: []string{"10.0.0.1"}}},
		Resources:   contract.Resources{Manifest: "10.0.0.1,22,root,pw"},
		RunMeta:     contract.RunMeta(sealed),
	}
}

// TestRunWritesReport 报告落在给的那条路径上，第一段吃的是解出来的原值，
// 节点清单那一行是遮过的内联记录。
func TestRunWritesReport(t *testing.T) {
	path := filepath.Join(t.TempDir(), "report.txt")
	if err := Run(ledgerForRun(t), path); err != nil {
		t.Fatalf("写报告失败：%v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("报告没落下：%v", err)
	}
	text := string(data)

	for _, want := range []string{
		"【执行命令】 \n  sshfleet2 -f nodes.csv -c uptime\n",
		"  执行模式： 命令模式\n",
		"  执行命令： uptime\n",
		"  节点清单： 10.0.0.1,22,root,****\n",
		"  节点数量： 1\n",
		"执行成功（1）：\n10.0.0.1\n",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("报告里缺了一句 %q\n全文：%s", want, text)
		}
	}
	if strings.Contains(text, "root,pw") {
		t.Fatalf("清单里的密码没遮就落盘了：%s", text)
	}
}

// TestRunRejectsBrokenRunMeta 运行上下文解不开就报出来：不猜、不拿别处的值顶，
// 也不留一份「看着对」的报告（15 号单「运行上下文的用法」）。
func TestRunRejectsBrokenRunMeta(t *testing.T) {
	l := ledgerForRun(t)
	l.RunMeta = contract.RunMeta{1, 2, 3}

	path := filepath.Join(t.TempDir(), "report.txt")
	if err := Run(l, path); err == nil {
		t.Fatal("解不开的运行上下文应当报错")
	}
	if _, err := os.Stat(path); err == nil {
		t.Fatal("解不开时不该落下一份报告：那是「拿别处的值顶」的另一种写法")
	}
}

// TestRunFailsWhenPathIsNotWritable 落点写不进去时把路径带回话里——报告站不自己找地方重写。
func TestRunFailsWhenPathIsNotWritable(t *testing.T) {
	dir := t.TempDir()
	if err := Run(ledgerForRun(t), filepath.Join(dir, "no-such-dir", "report.txt")); err == nil {
		t.Fatal("父目录不存在时应当报错")
	} else if !strings.Contains(err.Error(), "report.txt") {
		t.Fatalf("报错里没带上落点：%v", err)
	}
}
