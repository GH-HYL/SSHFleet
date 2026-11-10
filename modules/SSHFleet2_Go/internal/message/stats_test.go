package message

// 统计块与 `report.txt` 的逐字判据。
//
// 期望值**不是**照着本包代码重写一遍：可见的文字与数字来自老版真跑产物——
//
//	屏幕：build/建站04_15_2026-11-02_1515/screen_fail.txt / screen_ok.txt /
//	      screen_exit1_notips.txt（老版二进制对 u1 跑，输出重定向后落盘）
//	报告：build/建站04_12_2026-11-02_1440/ab_old/history/2026-11-02_14-46-26_command_exit/report.txt
//	      （1 台、`exit 3` 失败那一套）与
//	      build/建站04_10_2026-11-02_1245/ab/old/history/2026-11-02_13-00-38_command_uptime/report.txt
//	      （1 台、`uptime` 成功那一套）
//
// 两份时间戳按本用例里造的固定时刻写死；色码照老版统计块那几处（重定向时老版自己剥掉了，
// 所以录下来的文件里没有色码，这里按源码那几段补回）。老版屏幕与报告的那一处刻意不同
// （`>>>` 与 `-→`）也照录，不是笔误。

import (
	"strings"
	"testing"
	"time"

	"sshfleet2/internal/contract"
)

// 色码：与老版统计块同一套。写成字面量，不去引用本包那几个常量——期望值不该跟着被测代码走。
// 红黄与复位三个 screens_test.go 已经写过，这里只补统计块多用的三色。
const (
	escGreen = "\x1b[32m"
	escCyan  = "\x1b[36m"
	escDim   = "\x1b[90m"
)

const statsBar = "════════════════════════════════════════════════════════════"

// at 把报告里那种带微秒的时间戳写成本用例的固定时刻（本地时区，与老版同一条路）。
func at(s string) time.Time {
	tm, err := time.ParseInLocation("2006-01-02 15:04:05.000000", s, time.Local)
	if err != nil {
		panic(err)
	}
	return tm
}

// ledgerFail 是「1 台、exit 3」那一套的总账：与录下来的那次真跑同形。
func ledgerFail() contract.Ledger {
	code := 3
	return contract.Ledger{
		Total: 1, Done: 1, NotRun: 0, Verify: VerifyPassed,
		SuccessCounts: 0, FailCounts: 1,
		SuccessCategory: "执行成功",
		FailCategories:  []contract.CategoryCount{{Category: "执行失败(退出码3)", Count: 1, ExitCode: &code}},
		CategoryIPs:     []contract.CategoryIPs{{Category: "执行失败(退出码3)", IPs: []string{"127.0.0.1"}}},
		CostTime:        0.20,
		Concurrency:     1,
	}
}

// TestStatisticsBlockFailWithTips 对上 screen_fail.txt：`exit 3`、提示开关开着。
func TestStatisticsBlockFailWithTips(t *testing.T) {
	want := statsBar + "\n" +
		"  总耗时：0.20 秒\n" +
		"  " + escCyan + "节点总数：" + escReset + " 1  " + escCyan + "完成总数：" + escReset + "1\n" +
		"  " + escGreen + "成功：" + escReset + " 0   " + escRed + "失败：" + escReset + " 1\n" +
		"  " + escRed + "失败分类统计" + escReset + " >>>  " +
		escYellow + "执行失败(退出码3) ×" + escReset + "1\n" +
		"  " + escDim + "提示：" + escReset + "\n" +
		"    " + escDim + "执行失败(退出码3)：命令自身失败，或命令没跑起来被拒（未识别出具体原因）" + escReset + "\n" +
		statsBar + "\n"

	got := StatisticsLine(ledgerFail(), true, "config/keywords_error.conf").Text()
	if got != want {
		t.Fatalf("统计块与老版那一次对不上\n got=%q\nwant=%q", got, want)
	}
}

// TestStatisticsBlockSuccess 对上 screen_ok.txt：1 台成功，没有失败分类那一段。
func TestStatisticsBlockSuccess(t *testing.T) {
	l := contract.Ledger{
		Total: 1, Done: 1, Verify: VerifyPassed,
		SuccessCounts: 1, FailCounts: 0,
		SuccessCategory: "执行成功",
		CategoryIPs:     []contract.CategoryIPs{{Category: "执行成功", IPs: []string{"127.0.0.1"}}},
		CostTime:        0.12,
	}
	want := statsBar + "\n" +
		"  总耗时：0.12 秒\n" +
		"  " + escCyan + "节点总数：" + escReset + " 1  " + escCyan + "完成总数：" + escReset + "1\n" +
		"  " + escGreen + "成功：" + escReset + " 1\n" +
		statsBar + "\n"

	got := StatisticsLine(l, true, "config/keywords_error.conf").Text()
	if got != want {
		t.Fatalf("成功那一次的统计块对不上\n got=%q\nwant=%q", got, want)
	}
}

// TestStatisticsBlockTipsOff 对上 screen_exit1_notips.txt：关掉提示开关、退出码 1。
// 老版退回「只给一行常见退出码含义」。
func TestStatisticsBlockTipsOff(t *testing.T) {
	code := 1
	l := ledgerFail()
	l.FailCategories = []contract.CategoryCount{{Category: "执行失败(退出码1)", Count: 1, ExitCode: &code}}
	l.CategoryIPs = []contract.CategoryIPs{{Category: "执行失败(退出码1)", IPs: []string{"127.0.0.1"}}}
	l.CostTime = 0.10

	want := statsBar + "\n" +
		"  总耗时：0.10 秒\n" +
		"  " + escCyan + "节点总数：" + escReset + " 1  " + escCyan + "完成总数：" + escReset + "1\n" +
		"  " + escGreen + "成功：" + escReset + " 0   " + escRed + "失败：" + escReset + " 1\n" +
		"  " + escRed + "失败分类统计" + escReset + " >>>  " +
		escYellow + "执行失败(退出码1) ×" + escReset + "1\n" +
		"  " + escYellow + "常见退出码：1 >> 一般性错误" + escReset + "\n" +
		statsBar + "\n"

	got := StatisticsLine(l, false, "config/keywords_error.conf").Text()
	if got != want {
		t.Fatalf("关掉提示后那一屏对不上\n got=%q\nwant=%q", got, want)
	}
}

// TestStatisticsBlockTipsOffNoHints 同一个开关下，退出码不在含义表里（3）时不出那一行。
func TestStatisticsBlockTipsOffNoHints(t *testing.T) {
	got := StatisticsLine(ledgerFail(), false, "config/keywords_error.conf").Text()
	if strings.Contains(got, "常见退出码") {
		t.Fatalf("退出码 3 没有常见含义，不该出那一行：%q", got)
	}
	if strings.Contains(got, "提示：") {
		t.Fatalf("提示开关关着，不该出「提示：」那一段：%q", got)
	}
}

// TestStatisticsBlockUnclassified 判据表里没有的分类落到「未归类」，之后是那三行引导——
// 第三行里的路径来自运行上下文带的展示路径，不是本包自己算的。
func TestStatisticsBlockUnclassified(t *testing.T) {
	l := ledgerFail()
	l.FailCategories = []contract.CategoryCount{{Category: "密码过期", Count: 2}}
	got := StatisticsLine(l, true, "config/keywords_error.conf").Text()

	for _, want := range []string{
		"    " + escYellow + "密码过期 ×" + escReset + "2\n",
		"  " + escYellow + "未归类 1 类：" + escReset + "上方直接显示的是报错原文，没能匹配到已有分类。\n",
		"  若其中某类会反复出现，可把它的特征词补进 config/keywords_error.conf，\n",
		"  之后就会单独归为一类。\n",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("未归类那一段缺了一句 %q\n整屏：%q", want, got)
		}
	}
	// 未归类的那一类不能被当成「认得」的：认得的那一栏空了，只剩「>>>」。
	if !strings.Contains(got, "失败分类统计"+escReset+" >>>\n") {
		t.Fatalf("认得的那一栏该是空的：%q", got)
	}
}

// TestStatisticsBlockInterrupted 中断那一栏：把「未执行」单独列出，校验那一格不印。
func TestStatisticsBlockInterrupted(t *testing.T) {
	l := contract.Ledger{
		Total: 5, Done: 2, NotRun: 3, Interrupted: true, Verify: "",
		SuccessCounts: 2, FailCounts: 0, SuccessCategory: "执行成功", CostTime: 1.5,
	}
	got := StatisticsLine(l, true, "config/keywords_error.conf").Text()
	wantLine := "  " + escCyan + "节点总数：" + escReset + " 5  " + escCyan + "完成总数：" + escReset + "2  " +
		escCyan + "未执行：" + escReset + "3\n"
	if !strings.Contains(got, wantLine) {
		t.Fatalf("中断那一行不对\n got=%q\nwant 含=%q", got, wantLine)
	}
	if strings.Contains(got, "总数校验") {
		t.Fatalf("中断时不该印「总数校验」：%q", got)
	}
}

// reportBody 是报告用例的运行上下文：与那次真跑同形（`./sshfleet -f u1.csv -c 'exit 3' --yes`）。
func reportBody() contract.RunMetaBody {
	return contract.RunMetaBody{
		Argv:              []string{"./sshfleet", "-f", "u1.csv", "-c", "exit 3", "--yes"},
		Mode:              contract.ModeCommand,
		Command:           "exit 3",
		TimeoutConnect:    10,
		TimeoutExecute:    60,
		ShowCategoryTips:  true,
		ErrorKeywordsPath: "config/keywords_error.conf",
	}
}

// TestReportTextFail 对上那次 `exit 3` 真跑的 report.txt（时间换成本用例的固定时刻）。
func TestReportTextFail(t *testing.T) {
	l := ledgerFail()
	l.StartedAt = at("2026-11-02 14:46:26.900965")
	l.StoppedAt = at("2026-11-02 14:46:27.013470")
	l.CostTime = 0.11

	want := `=============================执行结果统计报告=============================
执行开始时间： 2026-11-02 14:46:26.900965
执行结束时间： 2026-11-02 14:46:27.013470

【执行命令】 
  ./sshfleet -f u1.csv -c 'exit 3' --yes

【执行参数】
  执行模式： 命令模式
  执行命令： exit 3
  节点清单： u1.csv
  节点数量： 1
  并发数： 1
  连接超时： 10s
  执行超时： 60s

【结果统计】
  总耗时：0.11 秒
  节点总数：1  完成总数：1  总数校验：通过
  成功：0    失败：1
  失败分类统计 -→  执行失败(退出码3) ×1
  提示：
    执行失败(退出码3)：命令自身失败，或命令没跑起来被拒（未识别出具体原因）

【节点清单统计】

执行失败(退出码3)（1）：
127.0.0.1
`

	got := ReportText(l, reportBody(), reportBody().Argv, "u1.csv", "")
	if got != want {
		t.Fatalf("report.txt 与老版那一次对不上\n got=%q\nwant=%q", got, want)
	}
}

// TestReportTextSuccess 对上那次 `uptime` 真跑的 report.txt。
func TestReportTextSuccess(t *testing.T) {
	l := contract.Ledger{
		Total: 1, Done: 1, Verify: VerifyPassed,
		SuccessCounts: 1, FailCounts: 0,
		SuccessCategory: "执行成功",
		CategoryIPs:     []contract.CategoryIPs{{Category: "执行成功", IPs: []string{"127.0.0.1"}}},
		StartedAt:       at("2026-11-02 13:00:38.127022"),
		StoppedAt:       at("2026-11-02 13:00:38.184830"),
		CostTime:        0.06,
	}
	body := reportBody()
	body.Argv = []string{"./sshfleet", "-c", "uptime", "-f", "u1.csv", "--yes", "--no-sudo", "--no-shell"}
	body.Command = "uptime"

	want := `=============================执行结果统计报告=============================
执行开始时间： 2026-11-02 13:00:38.127022
执行结束时间： 2026-11-02 13:00:38.184830

【执行命令】 
  ./sshfleet -c uptime -f u1.csv --yes --no-sudo --no-shell

【执行参数】
  执行模式： 命令模式
  执行命令： uptime
  节点清单： u1.csv
  节点数量： 1
  并发数： 1
  连接超时： 10s
  执行超时： 60s

【结果统计】
  总耗时：0.06 秒
  节点总数：1  完成总数：1  总数校验：通过
  成功：1    失败：0

【节点清单统计】

执行成功（1）：
127.0.0.1
`

	got := ReportText(l, body, body.Argv, "u1.csv", "")
	if got != want {
		t.Fatalf("成功那一次的 report.txt 对不上\n got=%q\nwant=%q", got, want)
	}
}

// TestReportTextTipsOff 关掉提示开关：报告里那一段也不写（老版同一个开关管两处）。
func TestReportTextTipsOff(t *testing.T) {
	l := ledgerFail()
	body := reportBody()
	body.ShowCategoryTips = false
	got := ReportText(l, body, body.Argv, "u1.csv", "")
	if strings.Contains(got, "提示：") {
		t.Fatalf("开关关着，报告里不该有「提示：」那一段：%q", got)
	}
}

// TestDisplayCommandQuotes 只做一件事：该加引号的加、能省的省。逗号在内联清单里是分隔符，
// 加引号会让报告里的写法失真（老版 shellSafeChars 的第一条理由）。
func TestDisplayCommandQuotes(t *testing.T) {
	cases := []struct {
		argv []string
		want string
	}{
		{[]string{"sshfleet2", "-c", "uptime"}, "sshfleet2 -c uptime"},
		{[]string{"sshfleet2", "-c", "exit 3"}, "sshfleet2 -c 'exit 3'"},
		{[]string{"sshfleet2", "-r", "a b"}, "sshfleet2 -r 'a b'"},
		// 遮过的那段带 `*`，`*` 会被 shell 展开，所以要引号；逗号本身不引——内联清单靠它分段。
		{[]string{"sshfleet2", "-f", "1.2.3.4,22,root,****"}, "sshfleet2 -f '1.2.3.4,22,root,****'"},
		{[]string{"sshfleet2", "-f", "10.0.0.1,22,root,pw"}, "sshfleet2 -f 10.0.0.1,22,root,pw"},
		{[]string{"sshfleet2", "-c", "echo 'hi'"}, `sshfleet2 -c 'echo '\''hi'\'''`},
		{[]string{"sshfleet2", ""}, "sshfleet2 ''"},
	}
	for _, c := range cases {
		if got := DisplayCommand(c.argv); got != c.want {
			t.Fatalf("argv=%v\n got=%q\nwant=%q", c.argv, got, c.want)
		}
	}
}

// 四支各自的整份报告（17 号单）：期望串是老版对同一份输入的真跑录，
// 在 build/建站05_17_2026-11-03_1415/old/rec/{script,upload,download,passwd}/report.txt。
// 换掉的只有两个时间戳与耗时——那两样每次都不同，用例里写死。
//
// 一份一份整文比，不按行挑：这四支的差别全在「执行参数」那几行上，按行挑等于只测那几行，
// 上下文的空行、段标题、末尾那一截照样可能被挤歪。

// TestReportTextScript 对上老版 `-s hello.sh` 那一次。
func TestReportTextScript(t *testing.T) {
	l := contract.Ledger{
		Total: 1, Done: 1, Verify: VerifyPassed,
		SuccessCounts: 1, FailCounts: 0,
		SuccessCategory: "执行成功",
		CategoryIPs:     []contract.CategoryIPs{{Category: "执行成功", IPs: []string{"127.0.0.1"}}},
		StartedAt:       at("2026-11-03 14:28:39.026129"),
		StoppedAt:       at("2026-11-03 14:28:39.144170"),
		CostTime:        0.12,
		Concurrency:     1,
	}
	body := contract.RunMetaBody{
		Argv:             []string{"./sshfleet", "-f", "u1.csv", "-s", "hello.sh", "--yes"},
		Mode:             contract.ModeScript,
		ScriptPath:       "hello.sh",
		TimeoutConnect:   10,
		TimeoutExecute:   60,
		ShowCategoryTips: false,
	}

	want := `=============================执行结果统计报告=============================
执行开始时间： 2026-11-03 14:28:39.026129
执行结束时间： 2026-11-03 14:28:39.144170

【执行命令】 
  ./sshfleet -f u1.csv -s hello.sh --yes

【执行参数】
  执行模式： 脚本模式
  脚本路径： hello.sh
  节点清单： u1.csv
  节点数量： 1
  并发数： 1
  连接超时： 10s
  执行超时： 60s

【结果统计】
  总耗时：0.12 秒
  节点总数：1  完成总数：1  总数校验：通过
  成功：1    失败：0

【节点清单统计】

执行成功（1）：
127.0.0.1
`

	if got := ReportText(l, body, body.Argv, "u1.csv", ""); got != want {
		t.Fatalf("脚本模式那份报告与老版对不上\n got=%q\nwant=%q", got, want)
	}
}

// TestReportTextUpload 对上老版 `-u ./updir -p /tmp/fleet-up/` 那一次。
func TestReportTextUpload(t *testing.T) {
	l := contract.Ledger{
		Total: 1, Done: 1, Verify: VerifyPassed,
		SuccessCounts: 1, FailCounts: 0,
		SuccessCategory: "传输成功",
		CategoryIPs:     []contract.CategoryIPs{{Category: "传输成功", IPs: []string{"127.0.0.1"}}},
		StartedAt:       at("2026-11-03 14:29:00.433515"),
		StoppedAt:       at("2026-11-03 14:29:00.646477"),
		CostTime:        0.21,
		Concurrency:     1,
	}
	body := contract.RunMetaBody{
		Argv:             []string{"./sshfleet", "-f", "u1.csv", "-u", "./updir", "-p", "/tmp/fleet-up/", "--yes"},
		Mode:             contract.ModeUpload,
		Source:           "./updir",
		Target:           "/tmp/fleet-up/",
		TimeoutConnect:   10,
		TimeoutTransfer:  300,
		ShowCategoryTips: false,
	}

	want := `=============================执行结果统计报告=============================
执行开始时间： 2026-11-03 14:29:00.433515
执行结束时间： 2026-11-03 14:29:00.646477

【执行命令】 
  ./sshfleet -f u1.csv -u ./updir -p /tmp/fleet-up/ --yes

【执行参数】
  执行模式： 上传模式
  本地路径： ./updir
  远程路径： /tmp/fleet-up/
  节点清单： u1.csv
  节点数量： 1
  并发数： 1
  连接超时： 10s
  传输超时： 300s

【结果统计】
  总耗时：0.21 秒
  节点总数：1  完成总数：1  总数校验：通过
  成功：1    失败：0

【节点清单统计】

传输成功（1）：
127.0.0.1
`

	if got := ReportText(l, body, body.Argv, "u1.csv", ""); got != want {
		t.Fatalf("上传模式那份报告与老版对不上\n got=%q\nwant=%q", got, want)
	}
}

// TestReportTextDownload 对上老版 `-d /tmp/fleet-up/one.txt -p ./down/` 那一次。
func TestReportTextDownload(t *testing.T) {
	l := contract.Ledger{
		Total: 1, Done: 1, Verify: VerifyPassed,
		SuccessCounts: 1, FailCounts: 0,
		SuccessCategory: "传输成功",
		CategoryIPs:     []contract.CategoryIPs{{Category: "传输成功", IPs: []string{"127.0.0.1"}}},
		StartedAt:       at("2026-11-03 14:28:39.297063"),
		StoppedAt:       at("2026-11-03 14:28:39.391718"),
		CostTime:        0.09,
		Concurrency:     1,
	}
	body := contract.RunMetaBody{
		Argv:             []string{"./sshfleet", "-f", "u1.csv", "-d", "/tmp/fleet-up/one.txt", "-p", "./down/", "--yes"},
		Mode:             contract.ModeDownload,
		Source:           "/tmp/fleet-up/one.txt",
		Target:           "./down/",
		TimeoutConnect:   10,
		TimeoutTransfer:  300,
		ShowCategoryTips: false,
	}

	want := `=============================执行结果统计报告=============================
执行开始时间： 2026-11-03 14:28:39.297063
执行结束时间： 2026-11-03 14:28:39.391718

【执行命令】 
  ./sshfleet -f u1.csv -d /tmp/fleet-up/one.txt -p ./down/ --yes

【执行参数】
  执行模式： 下载模式
  远程路径： /tmp/fleet-up/one.txt
  本地路径： ./down/
  节点清单： u1.csv
  节点数量： 1
  并发数： 1
  连接超时： 10s
  传输超时： 300s

【结果统计】
  总耗时：0.09 秒
  节点总数：1  完成总数：1  总数校验：通过
  成功：1    失败：0

【节点清单统计】

传输成功（1）：
127.0.0.1
`

	if got := ReportText(l, body, body.Argv, "u1.csv", ""); got != want {
		t.Fatalf("下载模式那份报告与老版对不上\n got=%q\nwant=%q", got, want)
	}
}

// TestReportTextPasswd 对上老版 `--change-password 'Fleet#New2026'` 那一次：
// 命令行与新密码两处都是遮过的形态（`Fl****26`），报告里不许出现明文。
func TestReportTextPasswd(t *testing.T) {
	l := contract.Ledger{
		Total: 1, Done: 1, Verify: VerifyPassed,
		SuccessCounts: 1, FailCounts: 0,
		SuccessCategory: "改密成功",
		CategoryIPs:     []contract.CategoryIPs{{Category: "改密成功", IPs: []string{"127.0.0.1"}}},
		StartedAt:       at("2026-11-03 14:28:39.593876"),
		StoppedAt:       at("2026-11-03 14:28:39.842087"),
		CostTime:        0.25,
		Concurrency:     1,
	}
	body := contract.RunMetaBody{
		Argv:             []string{"./sshfleet", "-f", "t1.csv", "--change-password", "Fl****26", "--yes"},
		Mode:             contract.ModePasswd,
		NewPassword:      "Fleet#New2026",
		TimeoutConnect:   10,
		TimeoutExecute:   60,
		ShowCategoryTips: false,
	}

	want := `=============================执行结果统计报告=============================
执行开始时间： 2026-11-03 14:28:39.593876
执行结束时间： 2026-11-03 14:28:39.842087

【执行命令】 
  ./sshfleet -f t1.csv --change-password 'Fl****26' --yes

【执行参数】
  执行模式： 改密模式
  新密码： Fl****26
  节点清单： t1.csv
  节点数量： 1
  并发数： 1
  连接超时： 10s
  改密超时： 60s

【结果统计】
  总耗时：0.25 秒
  节点总数：1  完成总数：1  总数校验：通过
  成功：1    失败：0

【节点清单统计】

改密成功（1）：
127.0.0.1
`

	got := ReportText(l, body, body.Argv, "t1.csv", "Fl****26")
	if got != want {
		t.Fatalf("改密模式那份报告与老版对不上\n got=%q\nwant=%q", got, want)
	}
	if strings.Contains(got, body.NewPassword) {
		t.Fatalf("报告里露出了新密码明文：%q", got)
	}
}

// TestReportTextAnswers 「代填 N」那几行：内容与**全部**触发词、序号从 1 起、留空印「（回车）」。
//
// 前两例各对一份老版真跑：`rec/answer-multi`（一条代填挂两个触发词）、`rec/answer-empty`
// （`-a ',Go on:'`，只发一个回车）。行位置也照录——排在节点数量之后、并发数之前。
func TestReportTextAnswers(t *testing.T) {
	cases := []struct {
		name    string
		argv    []string
		script  string
		answers []contract.Answer
		want    string
	}{
		{
			name:    "一条代填挂两个触发词（老版那次 -a 'y,Continue?,Press enter to go on:'）",
			argv:    []string{"./sshfleet", "-f", "u1.csv", "-s", "ask1.sh", "-a", "y,Continue?,Press enter to go on:", "--yes"},
			script:  "ask1.sh",
			answers: []contract.Answer{{Value: "y", Triggers: []string{"Continue?", "Press enter to go on:"}}},
			want: "  节点数量： 1\n" +
				"  代填 1： y → Continue?、Press enter to go on:\n" +
				"  并发数： 1\n",
		},
		{
			name:    "内容为空（老版那次 -a ',Go on:'）",
			argv:    []string{"./sshfleet", "-f", "u1.csv", "-s", "ask2.sh", "-a", ",Go on:", "--yes"},
			script:  "ask2.sh",
			answers: []contract.Answer{{Value: "", Triggers: []string{"Go on:"}}},
			want: "  节点数量： 1\n" +
				"  代填 1： （回车） → Go on:\n" +
				"  并发数： 1\n",
		},
		{
			name:   "两条按命令行顺序编号",
			argv:   []string{"./sshfleet", "-f", "u1.csv", "-s", "ask3.sh", "--yes"},
			script: "ask3.sh",
			answers: []contract.Answer{
				{Value: "y", Triggers: []string{"Continue?"}},
				{Value: "", Triggers: []string{"Go on:"}},
			},
			want: "  节点数量： 1\n" +
				"  代填 1： y → Continue?\n" +
				"  代填 2： （回车） → Go on:\n" +
				"  并发数： 1\n",
		},
	}
	for _, c := range cases {
		l := contract.Ledger{
			Total: 1, Done: 1, Verify: VerifyPassed,
			SuccessCounts: 1, FailCounts: 0,
			SuccessCategory: "执行成功",
			CategoryIPs:     []contract.CategoryIPs{{Category: "执行成功", IPs: []string{"127.0.0.1"}}},
			CostTime:        0.13,
			Concurrency:     1,
		}
		body := contract.RunMetaBody{
			Argv: c.argv, Mode: contract.ModeScript, ScriptPath: c.script,
			Answers:        c.answers,
			TimeoutConnect: 10, TimeoutExecute: 60,
		}

		if got := ReportText(l, body, body.Argv, "u1.csv", ""); !strings.Contains(got, c.want) {
			t.Fatalf("%s：代填那几行不对\n want 含=%q\n got=%q", c.name, c.want, got)
		}
	}
}

// TestReportTextConcurrencyFromLedger 并发数读**第三张纸**那一格：闸门问过之后可能改过，
// 运行上下文封的是改之前的值。纸上写着几就是几；纸上没写（0）才退回节点数。
func TestReportTextConcurrencyFromLedger(t *testing.T) {
	for _, c := range []struct {
		name  string
		n     int
		total int
		want  string
	}{
		{"闸门改成了 3", 3, 3, "  并发数： 3\n"},
		{"没指定 -n，与节点数同", 0, 1, "  并发数： 1\n"},
		// 纸上那一格比台数还大（闸门没封顶的老纸）：印实际能起的台数，不许印原值。
		{"纸上比台数大，印封顶后的台数", 5, 3, "  并发数： 3\n"},
	} {
		l := ledgerFail()
		l.Total = c.total
		l.Concurrency = c.n
		body := reportBody()
		body.TimeoutConnect = 0
		body.TimeoutExecute = 0

		if got := ReportText(l, body, body.Argv, "u1.csv", ""); !strings.Contains(got, c.want) {
			t.Fatalf("%s：报告里该有 %q\n全文：%q", c.name, c.want, got)
		}
	}
}

// TestStatisticsBlockTipFromPasswdTable 改密那张表里的分类名：提示语取得到，也不算「未归类」。
//
// 分类名是全局的键，不因为登记在哪张表里而改变。漏掉第三张表，这句提示就没了，那一台还会
// 被归进「未归类」，末尾那句引导又把人指向 keywords_error.conf——而它压根不在那儿。
func TestStatisticsBlockTipFromPasswdTable(t *testing.T) {
	l := ledgerFail()
	l.FailCategories = []contract.CategoryCount{{Category: "密码未过期", Count: 1}}
	l.CategoryIPs = []contract.CategoryIPs{{Category: "密码未过期", IPs: []string{"127.0.0.1"}}}
	l.Tables = contract.Tables{PasswdCategories: []contract.Category{
		{Name: "密码未过期", Tip: "本来就不用改——清单里只放要改的那几台最省事"},
	}}

	got := StatisticsLine(l, true, "config/keywords_error.conf").Text()

	if !strings.Contains(got, "密码未过期 ×") {
		t.Fatalf("分类名没进统计：%q", got)
	}
	if !strings.Contains(got, "密码未过期：本来就不用改——清单里只放要改的那几台最省事") {
		t.Fatalf("那张表里写的提示语没取出来：%q", got)
	}
	if strings.Contains(got, "未归类") {
		t.Fatalf("登记过的分类名不该被当成未归类：%q", got)
	}
}
