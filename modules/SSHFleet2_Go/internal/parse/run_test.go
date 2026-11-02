package parse

import (
	"testing"

	"sshfleet2/internal/contract"
)

// 认「要不要看帮助」：-h / --help / 空参三种触发；带值选项吃掉下一个 token；
// 不认识的选项当场停；-- 之后不算；位置参数不拦。
//
// 这些断言的落点从 helpRequested 换成了 scan——命令行只读一遍（16 号单 H9），
// 认帮助与认取值用的是同一次扫描。

func TestScanHelp(t *testing.T) {
	cases := []struct {
		name string
		argv []string
		want bool
	}{
		{"空参数", []string{"sshfleet"}, true},
		{"短帮助", []string{"sshfleet", "-h"}, true},
		{"长帮助", []string{"sshfleet", "--help"}, true},
		{"帮助在值后面", []string{"sshfleet", "-f", "a.csv", "-h"}, true},
		{"值恰好是 -h", []string{"sshfleet", "-c", "-h"}, false},
		{"带等号的取值里带 -h", []string{"sshfleet", "-c=-h"}, false},
		{"位置参数后面的帮助", []string{"sshfleet", "misc", "-h"}, true},
		{"双横线之后不算", []string{"sshfleet", "--", "-h"}, false},
		{"不认识的选项当场停", []string{"sshfleet", "--bogus", "-h"}, false},
		{"只有别的选项", []string{"sshfleet", "--yes"}, false},
		{"布尔开关后面的帮助", []string{"sshfleet", "--yes", "-h"}, true},
		{"值本身长得像选项但认得出", []string{"sshfleet", "--command", "-h"}, false},
		// H9 的落点：带等号的内联取值不再让后面的 -h 认不出来（老版 pflag 就是这个结局）。
		{"等号内联值之后还有 -h", []string{"sshfleet", "--command=ls", "-h"}, true},
		{"等号内联值之后还有 --help", []string{"sshfleet", "--command=ls", "--help"}, true},
		{"等号内联值之后是未知选项", []string{"sshfleet", "--command=ls", "--bogus"}, false},
	}
	for _, c := range cases {
		if got := scan(c.argv).help; got != c.want {
			t.Errorf("%s：want %v，got %v（argv %v）", c.name, c.want, got, c.argv)
		}
	}
}

// 扫到错就停：认不出的选项只记一条，不多记后面的东西；位置参数只记不拦。
func TestScanStopsAtFirstProblem(t *testing.T) {
	s := scan([]string{"sshfleet", "--bogus", "--also-bogus"})
	if len(s.problems) != 1 || s.problems[0].Kind != contract.ProblemUnknownOption {
		t.Fatalf("应当只记一条「认不出的选项」，实际 %v", s.problems)
	}

	s = scan([]string{"sshfleet", "misc", "-c", "ls"})
	if s.help || len(s.problems) != 0 || s.vals["--command"] != "ls" {
		t.Fatalf("位置参数不该打断扫描：help=%v problems=%v vals=%v", s.help, s.problems, s.vals)
	}
	if len(s.extras) != 1 || s.extras[0] != "misc" {
		t.Fatalf("位置参数只记不拦，应当记下 misc，实际 %v", s.extras)
	}
}

// 两个身份开关同给：解析站只记一条事实，纸上只留一个结论字段（RunParams.Sudo）。
//
// 这条钉的是「事实在解析站产生」——只测闸门的话，把 argv.go 里记事实那一段删掉，
// 闸门测试照样绿（它自己塞 Problem），线上就变成静默按后一个覆盖。
func TestSudoConflictRecordedAsProblem(t *testing.T) {
	dir := writeTempConfig(t)
	manifest := writeFile(t, dir, "ok.csv", "10.0.0.1,22,root,pw\n")

	cfg, err := load()
	if err != nil {
		t.Fatalf("临时配置读不通：%v", err)
	}

	both := scan([]string{"sshfleet2", "--sudo", "--no-sudo", "-c", "uptime", "-f", manifest})
	p, fail := buildPlan(cfg, both)
	if fail != nil {
		t.Fatalf("两个开关同给不该走到「起不来」：%v", fail.Body)
	}
	found := false
	for _, pr := range p.Problems {
		if pr.Kind == contract.ProblemFlagConflict {
			found = true
			if pr.Option != "--sudo --no-sudo" {
				t.Errorf("冲突那条要记下两个开关名，实际 %q", pr.Option)
			}
		}
	}
	if !found {
		t.Fatalf("两个开关同给应当记一条 ProblemFlagConflict，实际 Problems=%v", p.Problems)
	}

	onlyNo := scan([]string{"sshfleet2", "--no-sudo", "-c", "uptime", "-f", manifest})
	p, fail = buildPlan(cfg, onlyNo)
	if fail != nil {
		t.Fatalf("只给 --no-sudo 不该走到「起不来」：%v", fail.Body)
	}
	for _, pr := range p.Problems {
		if pr.Kind == contract.ProblemFlagConflict {
			t.Fatalf("只给一个开关不该记冲突，实际 Problems=%v", p.Problems)
		}
	}
	if p.Run.Sudo {
		t.Errorf("只给 --no-sudo 时结论应当是不用 sudo，实际 %v", p.Run.Sudo)
	}
}

// 多余的位置参数：一段都不许丢，而且要认出它跟在哪个选项的值后面。
//
// 「认出主」这件事只在解析站做得了——分辨「值里有空格没加引号」与「参数打多了」
// 要看原始 argv，那东西只在扫描那一步手上。这里只验「事实认得准」，拒是闸门的事。
func TestExtraArgsCollected(t *testing.T) {
	cases := []struct {
		name      string
		argv      []string
		wantOwner string
	}{
		{"跟在一个带空格的值后面", []string{"sshfleet2", "-c", "echo hi", "extra"}, "-c"},
		{"跟在短选项值后面", []string{"sshfleet2", "-p", "/opt/a", "extra"}, "-p"},
		{"多余段在最前，认不出主", []string{"sshfleet2", "misc", "-c", "ls"}, ""},
		{"双横线之后的两段", []string{"sshfleet2", "-c", "ls", "--", "a", "b"}, ""},
		{"没有多余段", []string{"sshfleet2", "-c", "ls"}, ""},
	}
	for _, c := range cases {
		s := scan(c.argv)
		if len(s.extras) == 0 {
			if c.name != "没有多余段" {
				t.Errorf("%s：应当收到多余段，实际一段都没有", c.name)
			}
			continue
		}
		if s.extraOwner != c.wantOwner {
			t.Errorf("%s：认出的主 want %q，实际 %q", c.name, c.wantOwner, s.extraOwner)
		}
	}

	// 两段以上要原样拼回去，不能只留第一段。
	if s := scan([]string{"sshfleet2", "-c", "ls", "--", "a", "b"}); len(s.extras) != 2 {
		t.Errorf("双横线之后的两段都要收下，实际 %v", s.extras)
	}
}

// 多余的段要变成纸上的一条事实（ProblemExtraArgs），闸门才有东西可拒。
//
// 顺带钉住次序：有多余段时不读清单——老版 pflag 解析完发现多余段就 return，
// 本机一个 os 调用都还没发出去。少了这条，`-c x extra` 会先报「清单读不到」。
func TestExtraArgsRecordedAsProblem(t *testing.T) {
	writeTempConfig(t)
	cfg, err := load()
	if err != nil {
		t.Fatalf("临时配置读不通：%v", err)
	}

	p, fail := buildPlan(cfg, scan([]string{"sshfleet2", "-c", "echo hi", "extra"}))
	if fail != nil {
		t.Fatalf("有多余段时不该走到「起不来」（应当交给闸门拒）：%v", fail.Body)
	}
	found := false
	for _, pr := range p.Problems {
		if pr.Kind == contract.ProblemExtraArgs {
			found = true
			if pr.Option != "-c" || pr.Raw != "extra" {
				t.Errorf("多余段那条要记下主与原样，实际 Option=%q Raw=%q", pr.Option, pr.Raw)
			}
		}
	}
	if !found {
		t.Fatalf("多余段应当记一条 ProblemExtraArgs，实际 Problems=%v", p.Problems)
	}
}

// 下发行的材料由解析站带上纸：解释器、`[env]`、语言开关。
//
// 执行站不回头读配置（ADR-0014 / ADR-0015），这三样不走这条路就到不了下发行；
// 少了它们，`sudo` 与解释器会 command not found、远端提示语也不会钉成英文。
func TestBuildPlanCarriesDownlinkMaterial(t *testing.T) {
	writeTempConfig(t)
	manifest := writeFile(t, t.TempDir(), "ok.csv", "10.0.0.1,22,root,pw\n")

	cfg, err := load()
	if err != nil {
		t.Fatalf("临时配置读不通：%v", err)
	}

	p, fail := buildPlan(cfg, scan([]string{"sshfleet2", "-c", "uptime", "-f", manifest}))
	if fail != nil {
		t.Fatalf("不该走到「起不来」：%v", fail.Body)
	}
	if p.Run.Interpreter != "bash" {
		t.Errorf("解释器应照配置带上纸，实际 %q", p.Run.Interpreter)
	}
	if got := p.Run.Env["PATH"]; got != "/usr/bin" {
		t.Errorf("`[env]` 应照配置带上纸，PATH=%q", got)
	}
	if p.Run.English {
		t.Errorf("临时配置写的是 english = false，纸上却成了 true")
	}
}
