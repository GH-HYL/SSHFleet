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

	// 帮助：-h、--help、一个参数都不给。老版在第 3 步就返回，后面一概不做。
	if helpRequested(argv) {
		rev, tm, dirty := buildInfo()
		screen := message.HelpScreen(name, versionString(version, rev, tm, dirty), console.Width(), helpEntries(cfg.Execution))
		return screen, nil, nil
	}

	plan, fail := buildPlan(cfg, argv)
	if fail != nil {
		return contract.Text{}, nil, fail
	}
	return contract.Text{}, plan, nil
}

// cannotStart 排一屏「起不来」：环节、外壳句与原因照老版那一处。
func cannotStart(where, head, body string) *contract.CannotStart {
	full := head
	if body != "" {
		full = head + "：" + strings.TrimRight(body, "\r\n")
	}
	return &contract.CannotStart{Where: where, Say: message.Fatal(where, full, log.ToolPath()), Body: full}
}

// helpRequested 认「要不要看帮助」。认法照 pflag 的语义收窄到本工具：从左往右扫，
// 带值选项吃掉下一个 token（所以 `-c "-h"` 不会被当成要看帮助）；不认识的选项当场停
// （老版那时就报错，轮不到帮助）；`--` 之后不再有选项；位置参数不拦。
func helpRequested(argv []string) bool {
	if len(argv) <= 1 {
		return true // 一个参数都不给：老版打帮助后以 0 退出
	}
	for i := 1; i < len(argv); i++ {
		tok := argv[i]
		switch {
		case tok == "--":
			return false
		case tok == "-h" || tok == "--help":
			return true
		case !strings.HasPrefix(tok, "-") || tok == "-":
			continue // 位置参数，不拦
		}
		switch kindOf(tok) {
		case optTakesValue:
			i++ // 下一个 token 是它的值，不是选项
		case optKnown:
			// 继续扫
		default:
			return false // 不认识的选项：老版当场报错
		}
	}
	return false
}

// optKind 一个 token 在选项表里的身份。
type optKind int

const (
	optUnknown    optKind = iota // 不在表里
	optKnown                     // 开关，不带值
	optTakesValue                // 带值，吃下一个 token
)

func kindOf(tok string) optKind {
	for _, o := range options {
		if tok == o.short || tok == o.long {
			if o.takesValue {
				return optTakesValue
			}
			return optKnown
		}
	}
	return optUnknown
}
