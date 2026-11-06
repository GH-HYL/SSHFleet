package report

// 两张表格：开关由密文里那两格决定，内容逐格对老版那张表的字段。
//
// 期望值一律写字面量（表头、某台的取值、行数），不拿被测代码再算一遍。

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/xuri/excelize/v2"

	"sshfleet2/internal/archive"
	"sshfleet2/internal/contract"
)

// runMetaXlsxVector 由 internal/parse 封出来的一段运行上下文：命令模式，两张表格的开关都开着。
const runMetaXlsxVector = "AUCntDEQNa8nhVTwbACy2PIH2y+4go9qXo9lU1tLniC3ZVmBVzS719aXCGiY1nKMeaQ5Sd2Oi07/dqSIRiSf+9cLivvtFCdYa04F4bbV4gItgZleTEJFo4DL0xZu2vMcO9GQRUo7Pp7kKEOlfV+qW9kOGGabMQARuFGxhBEiW8/RqN5b5hOJCtla0t2QY2hzMcOxxEdU16y/MHXxbS9QtyQGDA=="

// ledgerForTables 造一张两台的总账：一台成功带输出，一台连不上。
func ledgerForTables(t *testing.T) contract.Ledger {
	t.Helper()
	sealed, err := base64.StdEncoding.DecodeString(runMetaXlsxVector)
	if err != nil {
		t.Fatalf("样本本身解不开：%v", err)
	}
	code := 1
	return contract.Ledger{
		Total: 2, Done: 2, Verify: "通过",
		SuccessCounts: 1, FailCounts: 1, SuccessCategory: "执行成功",
		Items: []contract.NodeResult{
			{
				IP: "10.0.0.1", Port: "22", User: "ops", Connected: true,
				ConnectCost: 123 * time.Millisecond, Cost: 456 * time.Millisecond,
				Category: "执行成功", ExitCode: &code, Output: "行一\n行二",
				Transfer: contract.TransferStat{TotalFiles: 3, SuccessFiles: 3, TotalBytes: 2048},
			},
			{
				IP: "10.0.0.2", Port: "2201", User: "root", Connected: false,
				Category:  "连接被拒",
				ErrorText: "sshfleet.connect.refused: dial tcp: connection refused",
			},
		},
		Resources: contract.Resources{Manifest: "nodes.csv"},
		RunMeta:   contract.RunMeta(sealed),
	}
}

// tablesPaths 把当前目录挪到临时目录（链接建在那儿），落点在同一个目录里。
func tablesPaths(t *testing.T) archive.Paths {
	t.Helper()
	dir := t.TempDir()
	t.Chdir(dir)
	if err := os.MkdirAll(filepath.Join(dir, contract.HistoryDir), 0o755); err != nil {
		t.Fatalf("建历史根失败：%v", err)
	}
	assets := filepath.Join(dir, "assets")
	if err := os.MkdirAll(assets, 0o755); err != nil {
		t.Fatalf("建 assets 失败：%v", err)
	}
	return archive.Paths{
		Dir:         dir,
		Assets:      assets,
		Report:      filepath.Join(dir, "report.txt"),
		OutputXlsx:  filepath.Join(dir, archive.NameOutputXlsx),
		ResultsXlsx: filepath.Join(dir, archive.NameResultsXlsx),
	}
}

// 两张表都生成：表头逐字、行数对、某台的取值对。
func TestRunWritesBothTables(t *testing.T) {
	paths := tablesPaths(t)
	if _, err := Run(ledgerForTables(t), paths); err != nil {
		t.Fatalf("跑报告站失败：%v", err)
	}

	results, err := excelize.OpenFile(paths.ResultsXlsx)
	if err != nil {
		t.Fatalf("结果表没生成：%v", err)
	}
	defer func() { _ = results.Close() }()
	rows, err := results.GetRows("Results")
	if err != nil {
		t.Fatalf("读结果表失败：%v", err)
	}
	wantHeader := []string{"序号", "IP", "端口", "用户名", "连接成功", "退出码", "连接耗时",
		"执行耗时", "传输字节", "文件总数", "成功文件数", "失败文件数", "分类", "报错原文", "输出内容"}
	if len(rows) != 3 {
		t.Fatalf("结果表该是表头 + 两行，实际 %d 行：%v", len(rows), rows)
	}
	for i, h := range wantHeader {
		if rows[0][i] != h {
			t.Fatalf("第 %d 列表头是 %q，该是 %q", i+1, rows[0][i], h)
		}
	}
	if rows[1][0] != "1" || rows[1][1] != "10.0.0.1" || rows[1][2] != "22" || rows[1][3] != "ops" {
		t.Fatalf("第一台那行不对：%v", rows[1])
	}
	if rows[1][4] != "TRUE" || rows[1][5] != "1" || rows[1][12] != "执行成功" {
		t.Fatalf("第一台的连接/退出码/分类不对：%v", rows[1])
	}
	if rows[1][13] != "" {
		t.Fatalf("成功那台的报错原文该空着：%v", rows[1])
	}
	if rows[2][13] != "sshfleet.connect.refused: dial tcp: connection refused" {
		t.Fatalf("失败那台的报错原文不对：%v", rows[2])
	}

	term, err := excelize.OpenFile(paths.OutputXlsx)
	if err != nil {
		t.Fatalf("终端表没生成：%v", err)
	}
	defer func() { _ = term.Close() }()
	trows, err := term.GetRows("执行日志")
	if err != nil {
		t.Fatalf("读终端表失败：%v", err)
	}
	if len(trows) == 0 || trows[0][0] != "IP地址" || trows[0][1] != "事件类型" || trows[0][2] != "内容详情" {
		t.Fatalf("终端表表头不对：%v", trows)
	}
	// 第一台：连接 → 执行 → 两行输出 → 分类 → 分隔行；第二台：连接 → 执行 → 错误 → 分类 → 分隔行。
	// 读回来少一行：最后那条分隔行只有底色、三列都空，`GetRows` 把尾部空行裁掉了。
	if len(trows) != 1+6+5-1 {
		t.Fatalf("终端表行数不对（表头 1 + 第一台 6 + 第二台 5，尾部空行被裁）：%d\n%v", len(trows), trows)
	}
	if trows[1][1] != "连接: 成功 - 0.123s" || trows[2][1] != "执行: 成功 - 0.456s" {
		t.Fatalf("前两行不是连接与执行：%v", trows[1:3])
	}
	if trows[3][1] != "标准输出和错误输出" || trows[3][2] != "行一" {
		t.Fatalf("输出没逐行摊开：%v", trows[3])
	}
	if trows[7][1] != "连接: 失败 - 0.000s" || trows[9][1] != "错误" {
		t.Fatalf("第二台那几行不对（第 8 行该是连接、第 10 行该是错误）：%v", trows[7:])
	}
}

// 开关关着：两张表都不生成（这一段密文里两格是 false）。
func TestRunSkipsTablesWhenSwitchedOff(t *testing.T) {
	paths := tablesPaths(t)
	if _, err := Run(ledgerForRun(t), paths); err != nil {
		t.Fatalf("跑报告站失败：%v", err)
	}
	for _, p := range []string{paths.OutputXlsx, paths.ResultsXlsx} {
		if _, err := os.Stat(p); err == nil {
			t.Fatalf("开关关着却生成了 %s", p)
		}
	}
}

// 表格写不进去：不留半截错误、不中断，交回一句让人拼日志的话。
func TestRunKeepsGoingWhenTableCannotBeWritten(t *testing.T) {
	paths := tablesPaths(t)
	bad := filepath.Join(t.TempDir(), "no-such-dir", "results.xlsx")
	paths.ResultsXlsx = bad

	warns, err := Run(ledgerForTables(t), paths)
	if err != nil {
		t.Fatalf("表格失败不该中断本次收尾：%v", err)
	}
	if len(warns) != 1 || !strings.Contains(warns[0].Text(), archive.NameResultsXlsx) {
		t.Fatalf("该有一句点名 results.xlsx 的留痕：%v", warns)
	}
	if _, err := os.Stat(paths.Report); err != nil {
		t.Fatalf("报告仍该写出来：%v", err)
	}
}

// 备份原料：清单以脱敏副本落进 assets/，文件名不变（脚本与代填由归档包原样复制，
// 各自那三条用例在 archive／mask 里）。
func TestRunBacksUpManifestCopy(t *testing.T) {
	paths := tablesPaths(t)
	manifest := filepath.Join(paths.Dir, "nodes.csv")
	if err := os.WriteFile(manifest, []byte("10.0.0.1,22,root,hunter2\n"), 0o644); err != nil {
		t.Fatalf("造清单失败：%v", err)
	}
	l := ledgerForTables(t)
	l.Resources.Manifest = manifest

	if _, err := Run(l, paths); err != nil {
		t.Fatalf("跑报告站失败：%v", err)
	}

	data, err := os.ReadFile(filepath.Join(paths.Assets, "nodes.csv"))
	if err != nil {
		t.Fatalf("清单副本没落进 assets：%v", err)
	}
	if string(data) != "10.0.0.1,22,root,hu****r2\n\n" {
		t.Fatalf("副本内容不对：%q", data)
	}
}
