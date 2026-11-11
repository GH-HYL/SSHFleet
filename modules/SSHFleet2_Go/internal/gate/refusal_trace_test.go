package gate

import (
	"strings"
	"testing"

	"sshfleet2/internal/contract"
	"sshfleet2/internal/message"
)

// 拒绝也要留痕（审计 a3 那处差异）。
//
// 判的是**两格不是一样东西**：`Say()` 是给人当场看的那一屏（红头、环节标签、末尾报日志
// 路径），`Trace()` 是事后翻日志的那一行（只有正文）。日志包自己补 `[cli] ` 前缀并按老版
// 规矩缩进续行，所以 trace 里多一个字都是叠壳，少一个字就是丢事实。
//
// 期望值向 `message` 现要，不手抄——抄一遍等于把「文案逐字照老版」这条判据换个地方再写一次。

// traceOf 走站入口（递纸收拒绝），拿这一拒报给日志的正文。
func traceOf(t *testing.T, p *contract.Plan) string {
	t.Helper()
	r, _ := Run(p)
	if r == nil {
		t.Fatal("这一张纸应当被拒，却放行了")
	}
	return r.Trace().Plain()
}

func TestRefusalTraceIsBodyOnly(t *testing.T) {
	cases := []struct {
		name string
		plan *contract.Plan
		want string
	}{
		{
			name: "认不出的选项（a3 那一场）",
			plan: planOf(contract.ModeCommand, func(p *contract.Plan) {
				p.Task.Command = &contract.CommandTask{Text: "ls"}
				p.Problems = append(p.Problems, contract.Problem{
					Kind: contract.ProblemUnknownOption, Option: "--nope",
				})
			}),
			want: message.RefuseUnknownOption("--nope"),
		},
		{
			name: "带值选项缺值",
			plan: planOf(contract.ModeNone, func(p *contract.Plan) {
				p.Problems = append(p.Problems, contract.Problem{
					Kind: contract.ProblemMissingValue, Option: "-c",
				})
			}),
			want: message.RefuseMissingValue("-c"),
		},
		{
			name: "合规那一段（模式零个）",
			plan: planOf(contract.ModeNone, nil),
			want: message.RefuseModeNone(),
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := traceOf(t, c.plan)
			if got != c.want {
				t.Fatalf("日志正文应当正好是那一句：\n要：%q\n得：%q", c.want, got)
			}
			for _, bad := range []string{"[ERROR]", "[function:", "日志：", "\x1b["} {
				if strings.Contains(got, bad) {
					t.Fatalf("日志正文里不该有 %q——外壳与路径由日志包/塔尖补：%q", bad, got)
				}
			}
		})
	}
}

// 多行的拒绝一字不改地交出去：续行缩进是日志包的活（老版 `logErrorLines` 与新版
// `log.Fatal` 都按「续行缩进两格、前缀仍带环节标签」办），闸门不许在这里预先拼好。
func TestRefusalTraceKeepsMultiLineBody(t *testing.T) {
	plan := planOf(contract.ModeCommand, func(p *contract.Plan) {
		p.Task.Command = &contract.CommandTask{Text: "uptime"}
		p.Problems = append(p.Problems, contract.Problem{
			Kind: contract.ProblemExtraArgs, Option: "-c", Raw: "uptime x",
		})
	})
	got := traceOf(t, plan)
	if !strings.Contains(got, "\n") {
		t.Fatalf("这一条正文本身是多行的，不该被压成一行：%q", got)
	}
	if got != message.RefuseExtraArgs("-c", "uptime x") {
		t.Fatalf("多行正文被改写了：\n要：%q\n得：%q", message.RefuseExtraArgs("-c", "uptime x"), got)
	}
}

// 「使用者自己喊停」不是失败：这类拒绝的 trace 必须为空，塔尖据此不落日志——
// 老版在 ErrCancelled 那条路上也是打完屏直接退，一个字都不写。
func TestCancelRefusalHasNoTrace(t *testing.T) {
	r := newRefusal(message.CancelLine())
	if r == nil {
		t.Fatal("造不出拒绝")
	}
	if got := r.Trace().Plain(); got != "" {
		t.Fatalf("答否不该留痕，trace 应当为空，得：%q", got)
	}
	// 屏上那句话照样要有——不留痕不等于不打屏。
	if r.Say().Plain() == "" {
		t.Fatal("答否那一行仍要上屏")
	}
}
