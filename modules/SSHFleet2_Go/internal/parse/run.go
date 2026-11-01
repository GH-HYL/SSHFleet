package parse

import (
	"path/filepath"
	"strings"

	"sshfleet2/internal/console"
	"sshfleet2/internal/contract"
	"sshfleet2/internal/log"
	"sshfleet2/internal/message"
)

// 解析站对外的唯一口子（出口白名单：至多一个函数，ADR-0037 决定三）。
//
// 它递回三样里的某一样：一屏话（帮助）、一张纸（进流水线）、一个「起不来」。
// 三样的组合就是塔尖认的结局，塔尖不自己判「这次该干嘛」。

// Run 吃整条 argv 与那行版本常量，吐「一屏话 + 一张纸 + 是不是起不来」。
//
// 配置、判据表、终端宽度都由本站自己取——塔尖除了 argv 和号，什么都不递
// （ADR-0032 第五条：它只搬，不看配置、不看文件在不在）。
func Run(argv []string, version string) (contract.Text, *contract.Plan, *contract.CannotStart) {
	cfg, err := load()
	if err != nil {
		// 配置这一步失败时日志还没建起来（老版也是这个先后），所以那一屏不带「日志：」行。
		body := "加载配置文件失败（" + shownConfigPath + "）：" + strings.TrimRight(err.Error(), "\r\n")
		return contract.Text{}, nil, &contract.CannotStart{
			Where: "config",
			Say:   message.Fatal("config", body, ""),
			Body:  body,
		}
	}

	name := "sshfleet2"
	if len(argv) > 0 {
		name = filepath.Base(argv[0])
	}

	// 命令行只扫一遍：帮助、取值、认不出的选项一起收下（16 号单 H9）。
	s := scan(argv)

	// 帮助：-h、--help、一个参数都不给。老版在第 3 步就返回，后面一概不做。
	if s.help {
		rev, tm, dirty := buildInfo()
		screen := message.HelpScreen(name, versionString(version, rev, tm, dirty), console.Width(), helpEntries(cfg.Execution))
		return screen, nil, nil
	}

	plan, fail := buildPlan(cfg, s)
	if fail != nil {
		return contract.Text{}, nil, fail
	}
	return contract.Text{}, plan, nil
}

// cannotStart 排一屏「起不来」：环节、外壳句与原因照老版那一处。
func cannotStart(where, head, body string) *contract.CannotStart {
	// head 为空表示老版那一处没有外壳句（判据表坏掉的三处就是这样），此时不补冒号。
	full := strings.TrimRight(body, "\r\n")
	if head != "" {
		full = head + "：" + full
	}
	return &contract.CannotStart{Where: where, Say: message.Fatal(where, full, log.ToolPath()), Body: full}
}
