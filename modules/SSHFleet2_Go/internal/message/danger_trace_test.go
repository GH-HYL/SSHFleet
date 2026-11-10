package message

import (
	"strings"
	"testing"

	"sshfleet2/internal/contract"
)

// 危险命令的两句留痕（2026-11-10，审计 08-4）：
// 交互答「是」写「用户已确认风险继续执行：」，非交互写「非交互模式放行危险命令：」。
// 两句必须分开——一句说「有人把关、他点了头」，一句说「无人值守放行」，事后追责不能混。
func TestDangerTraceWording(t *testing.T) {
	m := contract.DangerMatch{
		RuleName: "删除操作",
		Level:    "HIGH",
		Content:  "rm -rf /tmp/x",
		Example:  "rm -rf /path",
		Reason:   "删除目录",
		Lines:    []int{2},
	}

	confirmed := TraceDangerConfirmed(m).Text()
	if !strings.HasPrefix(confirmed, "用户已确认风险继续执行：") {
		t.Fatalf("交互答是那一句的外壳不对：%q", confirmed)
	}
	nonInteractive := TraceDanger(m).Text()
	if !strings.HasPrefix(nonInteractive, "非交互模式放行危险命令：") {
		t.Fatalf("--yes 那一句的外壳不对：%q", nonInteractive)
	}

	// 两句只在外壳上分岔：外壳之后那一段事实两边必须逐字相同，否则事实会漂成两份。
	note := dangerNote(m)
	if confirmed != "用户已确认风险继续执行："+note {
		t.Fatalf("交互那一句的事实段与共用拼法不一致：%q", confirmed)
	}
	if nonInteractive != "非交互模式放行危险命令："+note {
		t.Fatalf("--yes 那一句的事实段与共用拼法不一致：%q", nonInteractive)
	}

	// 事实段该有的东西一样不许少：规则名、级别、来源行、原文、示例、说明。
	for _, want := range []string{"删除操作", "HIGH", "来源行 2", "rm -rf /tmp/x", "示例：rm -rf /path", "说明：删除目录"} {
		if !strings.Contains(note, want) {
			t.Fatalf("事实段少了 %q：%q", want, note)
		}
	}
}
