package parse

import (
	"sshfleet2/internal/message"
	"strings"
	"testing"

	"sshfleet2/internal/contract"
)

// 第四种结局：解析站把「一条小命令」递回塔尖，而不是纸，也不是起不来。
//
// 三层判据各一条：三条命令各递回一次（哪条、值、--yes）；两条一起给、与批量参数一起给
// 当场是起不来；词法错时仍走闸门那条路（不进密钥分支）。

func TestKeyRunHandedBack(t *testing.T) {
	t.Chdir(writeTempConfig(t))
	cfg, err := load()
	if err != nil {
		t.Fatalf("配置读不通：%v", err)
	}

	cases := []struct {
		name  string
		argv  []string
		which contract.KeyTool
		path  string
		yes   bool
	}{
		{"生成", []string{"sshfleet2", "--gen-key"}, contract.KeyToolGen, "", false},
		{"看状态", []string{"sshfleet2", "--key-status"}, contract.KeyToolStatus, "", false},
		{"转换", []string{"sshfleet2", "--convert-secret", "sec.txt"}, contract.KeyToolConvert, "sec.txt", false},
		{"转换 + 非交互", []string{"sshfleet2", "--convert-secret", "a b.txt", "--yes"}, contract.KeyToolConvert, "a b.txt", true},
	}
	for _, c := range cases {
		plan, krun, fail := buildPlan(cfg, scan(c.argv))
		if fail != nil {
			t.Fatalf("%s：不该起不来：%+v", c.name, fail)
		}
		if plan != nil {
			t.Fatalf("%s：密钥命令不该建纸", c.name)
		}
		if krun == nil {
			t.Fatalf("%s：该递回一条小命令", c.name)
		}
		if krun.Which != c.which || krun.Path != c.path || krun.Yes != c.yes {
			t.Fatalf("%s：递回的事实不对：%+v", c.name, *krun)
		}
	}
}

func TestKeyRunRefusesTwoTools(t *testing.T) {
	t.Chdir(writeTempConfig(t))
	cfg, err := load()
	if err != nil {
		t.Fatalf("配置读不通：%v", err)
	}

	plan, krun, fail := buildPlan(cfg, scan([]string{"sshfleet2", "--gen-key", "--key-status"}))
	if plan != nil || krun != nil {
		t.Fatalf("不合规时不该递回纸或小命令")
	}
	if fail == nil {
		t.Fatalf("两条密钥命令一起给应当是起不来")
	}
	if fail.Where != "cli" {
		t.Fatalf("环节名是 cli，got %q", fail.Where)
	}
	for _, want := range []string{
		"选项/参数合规性检查未通过：",
		"--gen-key、--key-status 只能给一个——它们是三个独立命令，一次做一件事",
	} {
		if !strings.Contains(message.CannotStartText(*fail), want) {
			t.Fatalf("那一屏缺「%s」：%q", want, message.CannotStartText(*fail))
		}
	}
}

func TestKeyRunRefusesWithBatchParams(t *testing.T) {
	t.Chdir(writeTempConfig(t))
	cfg, err := load()
	if err != nil {
		t.Fatalf("配置读不通：%v", err)
	}

	_, _, fail := buildPlan(cfg, scan([]string{"sshfleet2", "--key-status", "-c", "whoami", "-n", "2"}))
	if fail == nil {
		t.Fatalf("密钥命令与批量参数一起给应当是起不来")
	}
	// 老版把「没给 -r 时的自动备注」也算作给了，所以这句里会看到 -r。
	for _, want := range []string{
		"--key-status 不能和批量执行选项一起用：同时给了 -c、-n、-r",
		"提示：分成两次执行——先跑密钥管理命令，再跑批量执行",
	} {
		if !strings.Contains(message.CannotStartText(*fail), want) {
			t.Fatalf("那一屏缺「%s」：%q", want, message.CannotStartText(*fail))
		}
	}
}

// 词法错时不进密钥分支：仍是纸（问题记在纸上），由闸门拒。
func TestKeyRunStaysOnPaperWhenLexicalError(t *testing.T) {
	t.Chdir(writeTempConfig(t))
	cfg, err := load()
	if err != nil {
		t.Fatalf("配置读不通：%v", err)
	}

	plan, krun, fail := buildPlan(cfg, scan([]string{"sshfleet2", "--zzz", "--key-status"}))
	if fail != nil || krun != nil {
		t.Fatalf("词法错时该交纸，不该起不来、也不该递小命令")
	}
	if plan == nil || len(plan.Problems) == 0 || plan.Problems[0].Kind != contract.ProblemUnknownOption {
		t.Fatalf("纸上该记一条「认不出的选项」：%+v", plan)
	}
}

// 多余的位置参数：老版在解析命令行那一步就报「出现多余的参数」，比密钥命令那两条互斥更早。
// 密钥命令不进闸门，这条不在这儿拦就没人拦了（批量执行那条路仍交纸给闸门）。
func TestKeyRunRefusesExtraArgs(t *testing.T) {
	t.Chdir(writeTempConfig(t))
	cfg, err := load()
	if err != nil {
		t.Fatalf("配置读不通：%v", err)
	}

	for _, argv := range [][]string{
		{"sshfleet2", "--key-status", "extra"},
		{"sshfleet2", "--gen-key", "--key-status", "extra"}, // 多余段比互斥更早，与老版同序
	} {
		plan, krun, fail := buildPlan(cfg, scan(argv))
		if plan != nil || krun != nil {
			t.Fatalf("%v：多余参数时不该递回纸或小命令", argv)
		}
		if fail == nil || fail.Where != "cli" {
			t.Fatalf("%v：应当是 cli 环的起不来，实际 %+v", argv, fail)
		}
		for _, want := range []string{
			"选项/参数解析失败：出现多余的参数（未被使用）：extra",
			"常见原因：",
		} {
			say := message.CannotStartText(*fail)
			if !strings.Contains(say, want) {
				t.Fatalf("%v：那一屏缺「%s」：%q", argv, want, say)
			}
		}
	}
}

// --convert-secret 给一个空串等于没给：仍按批量那一路走（老版 a.ConvertSecret != ""）。
func TestKeyConvertEmptyValueIsNotAKeyRun(t *testing.T) {
	t.Chdir(writeTempConfig(t))
	cfg, err := load()
	if err != nil {
		t.Fatalf("配置读不通：%v", err)
	}

	plan, krun, fail := buildPlan(cfg, scan([]string{"sshfleet2", "--convert-secret", ""}))
	if fail != nil || krun != nil || plan == nil {
		t.Fatalf("空值应当当作没给：plan=%v krun=%v fail=%v", plan, krun, fail)
	}
}
