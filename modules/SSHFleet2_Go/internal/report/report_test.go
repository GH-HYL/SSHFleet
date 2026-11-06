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

	"sshfleet2/internal/archive"
	"sshfleet2/internal/contract"
)

// runPaths 造一份落点并把当前目录挪到临时目录：收尾那条 `latest_history` 建在当前目录，
// 挪过去才不会往仓库里落链接。历史根建一个空的，链接那一步就什么都不做。
func runPaths(t *testing.T) archive.Paths {
	t.Helper()
	dir := t.TempDir()
	t.Chdir(dir)
	if err := os.MkdirAll(contract.HistoryDir, 0o755); err != nil {
		t.Fatalf("建历史根失败：%v", err)
	}
	return archive.Paths{Report: filepath.Join(dir, "report.txt")}
}

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
	paths := runPaths(t)
	if _, err := Run(ledgerForRun(t), paths); err != nil {
		t.Fatalf("写报告失败：%v", err)
	}

	data, err := os.ReadFile(paths.Report)
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

// runMetaPasswdVector 由 internal/parse 封出来的一段改密运行上下文：argv 里写着新密码明文、
// 纸上「新密码」那一格也装的是明文（`Fleet#New2026`）。
const runMetaPasswdVector = "ARJMOSlA116ow4kWGkIc/qyuKWyVcI5CgXXQzw+SIzhxQEkby27/JSUibqGBvjqUX8uWxP39RlF4prbyvhxNNWRC9QSrSVNzFDKCYZyuoGMLh7WdEAA41X2dQli9VRbDp4qCwW+/pW6n894EC6jVZeWJxMtNGb+TibxLdpQTn/t372p9SLiN+wUTUZpMi/zmZW1UtjNgSgHx7ceilSZa0CuxcYPF+OfniE/+NnVbVg9nUUpoL/SRR+cez1dPnynUzhk35BGu"

// TestRunMasksNewPassword 改密模式：密文里装的是**明文**新密码，报告站印之前自己调
// `mask` 现算一次。遮不是一格字段——纸上不带遮好的形态，谁要印谁算。
//
// 那一串同样由 internal/parse 封出来（改密模式、新密码 `Fleet#New2026`），本用例不自己封：
// 自己封等于把被封的东西也验一遍，验不到报告站这一侧。
func TestRunMasksNewPassword(t *testing.T) {
	sealed, err := base64.StdEncoding.DecodeString(runMetaPasswdVector)
	if err != nil {
		t.Fatalf("样本本身解不开：%v", err)
	}
	l := contract.Ledger{
		Total: 1, Done: 1, Verify: "通过",
		SuccessCounts: 1, SuccessCategory: "改密成功",
		CategoryIPs: []contract.CategoryIPs{{Category: "改密成功", IPs: []string{"127.0.0.1"}}},
		Resources:   contract.Resources{Manifest: "t1.csv"},
		RunMeta:     contract.RunMeta(sealed),
	}

	paths := runPaths(t)
	if _, err := Run(l, paths); err != nil {
		t.Fatalf("写报告失败：%v", err)
	}
	data, err := os.ReadFile(paths.Report)
	if err != nil {
		t.Fatalf("报告没落下：%v", err)
	}
	text := string(data)

	if !strings.Contains(text, "  新密码： Fl****26\n") {
		t.Fatalf("新密码那一行没遮：%s", text)
	}
	if strings.Contains(text, "Fleet#New2026") {
		t.Fatalf("报告里出现了新密码明文：%s", text)
	}
}

// TestRunRejectsBrokenRunMeta 运行上下文解不开就报出来：不猜、不拿别处的值顶，
// 也不留一份「看着对」的报告（15 号单「运行上下文的用法」）。
func TestRunRejectsBrokenRunMeta(t *testing.T) {
	l := ledgerForRun(t)
	l.RunMeta = contract.RunMeta{1, 2, 3}

	paths := runPaths(t)
	if _, err := Run(l, paths); err == nil {
		t.Fatal("解不开的运行上下文应当报错")
	}
	if _, err := os.Stat(paths.Report); err == nil {
		t.Fatal("解不开时不该落下一份报告：那是「拿别处的值顶」的另一种写法")
	}
}

// TestRunFailsWhenPathIsNotWritable 落点写不进去时把路径带回话里——报告站不自己找地方重写。
func TestRunFailsWhenPathIsNotWritable(t *testing.T) {
	paths := runPaths(t)
	paths.Report = filepath.Join(t.TempDir(), "no-such-dir", "report.txt")
	if _, err := Run(ledgerForRun(t), paths); err == nil {
		t.Fatal("父目录不存在时应当报错")
	} else if !strings.Contains(err.Error(), "report.txt") {
		t.Fatalf("报错里没带上落点：%v", err)
	}
}
