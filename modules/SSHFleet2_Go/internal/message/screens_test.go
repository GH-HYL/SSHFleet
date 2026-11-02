package message

import (
	"strings"
	"testing"

	"sshfleet2/internal/contract"
)

// 三套屏的形态（自定义区）：期望串逐字写死，形态取自老工程实跑的输出。
//
// 颜色在 message 这一层一律带着——去不去由 console 按流与 VT 位决定（判据在那边有测试）。
// 所以这里的字符串里必须看得到上色码，看不到才说明拼法被谁改掉了。

const (
	escRed    = "\x1b[31m"
	escYellow = "\x1b[33m"
	escReset  = "\x1b[0m"
)

// 起不来那一屏：红 [ERROR]、黄环节标签、正文，末尾附日志路径。
func TestFatalScreen(t *testing.T) {
	got := Fatal("cli", ProblemHint("问题一", "提示一"), `D:\logs\tool.log`).Text()
	want := escRed + "[ERROR]" + escReset + escYellow + " [function:cli]" + escReset +
		" 问题一\n提示：提示一\n日志：D:\\logs\\tool.log\n"
	if got != want {
		t.Fatalf("起不来那一屏不对：\n got  %q\n want %q", got, want)
	}
	if !Fatal("cli", "x", "").Stderr() {
		t.Fatalf("起不来要写标准错误")
	}
	// 日志还没建起来时不出现「日志：」那一行。
	if strings.Contains(Fatal("config", "x", "").Text(), "日志：") {
		t.Fatalf("没有日志句柄时不该有「日志：」行")
	}
}

// 三种三段话：问题+原因+提示、问题+原因、问题+提示、问题+出处。
func TestProblemShapes(t *testing.T) {
	cases := []struct{ got, want string }{
		{Problem("问题", "原因", "提示"), "问题\n原因：原因\n提示：提示"},
		{ProblemReason("问题", "原因"), "问题\n原因：原因"},
		{ProblemHint("问题", "提示"), "问题\n提示：提示"},
		{ProblemOrigin("问题", "出处"), "问题\n出处：出处"},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Fatalf("三段话拼错了：\n got  %q\n want %q", c.got, c.want)
		}
	}
}

// 拒绝那几屏：外壳照老版，走哪条流跟着句子走。
func TestRefuseScreens(t *testing.T) {
	// 输入不合规：外壳 + 两行，交出去时由闸门套 Fatal 并写 stderr。
	if got, want := RefusePathWithCommand(),
		"选项/参数合规性检查未通过：-p 参数不能搭配 -c 或 -s 使用\n提示：只有上传、下载才用 -p；命令与脚本模式不需要它"; got != want {
		t.Fatalf("拒绝那一屏不对：\n got  %q\n want %q", got, want)
	}
	// 身份开关那句只有一行、不补提示（老版如此）。
	if got, want := RefuseSudoConflict(),
		"选项/参数合规性检查未通过：--sudo 与 --no-sudo 只能给一个（两个都不给=按配置里的值）"; got != want {
		t.Fatalf("身份冲突那句不对：\n got  %q\n want %q", got, want)
	}
	// 模式零个：提示是三行，第三行带缩进的选项清单。
	if !strings.HasSuffix(RefuseModeNone(),
		"提示：你没有指定任何模式，请从下面五个里选一个：\n"+
			"      -c 执行命令   -s 执行脚本   -u 上传文件   -d 下载文件   --change-password 批量改密") {
		t.Fatalf("模式零个那屏不对：%q", RefuseModeNone())
	}

	// 禁止档与喊停走标准输出（重定向时这个差别看得见）。
	report := contract.DangerReport{Matches: []contract.DangerMatch{{
		RuleName: "递归删除根目录", Level: contract.RiskForbidden, Content: "rm -rf /", Lines: []int{1},
	}}}
	if DangerBox(report, true).Stderr() {
		t.Fatalf("禁止命令那张框走标准输出")
	}
	if CancelLine().Stderr() || InputEndLine().Stderr() || ConfirmPrompt().Stderr() {
		t.Fatalf("喊停那几句走标准输出")
	}
	if !strings.Contains(CancelLine().Text(), "操作已取消") || !strings.Contains(InputEndLine().Text(), "输入结束") {
		t.Fatalf("喊停的句子被改过了：%q / %q", CancelLine().Text(), InputEndLine().Text())
	}
}
