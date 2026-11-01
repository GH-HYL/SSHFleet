package parse

import (
	"path/filepath"
	"strings"

	"sshfleet2/internal/console"
	"sshfleet2/internal/contract"
	"sshfleet2/internal/message"
)

// 解析站对外的唯一口子（出口白名单：至多一个函数，ADR-0037 决定三）。
// 本阶段只认「要不要看帮助」：-h、--help、一个参数都不给。

// Run 塔尖把整条 argv 与那行版本常量递进来，拿回「一句要打给使用者的话」
// 加「是不是起不来」（nil 表示不是）。配置与终端宽度由本站自己取——塔尖除了
// argv 和号，什么都不递（ADR-0032 第五条：它只搬，不看配置、不看文件在不在）。
func Run(argv []string, version string) (contract.Text, *contract.CannotStart) {
	name := "sshfleet2"
	if len(argv) > 0 {
		name = filepath.Base(argv[0])
	}
	if helpRequested(argv) {
		return helpScreen(name, version)
	}
	// 帮助之外的用法本阶段没接上：一律「起不来」。
	// 这是过渡形态（见阶段规格 Out of Scope），随阶段 03、04 被真正的解析与合规检查取代。
	return message.CannotStartLine(
		"这条用法还没接上",
		"本阶段只认 -h / --help / 空参数；其余选项随阶段 03、04 接线",
	), &contract.CannotStart{}
}

// helpScreen 出帮助那一屏：配置四格 + 选项表 + 版本串 + 终端宽度，交给 message 排。
// 配置读不到、读不懂都是「起不来」——一句话加非 0 退出（话术随阶段 03 定稿）。
func helpScreen(name, num string) (contract.Text, *contract.CannotStart) {
	raw, err := readRawConfig()
	var f execution
	if err == nil {
		f, err = executionOf(raw)
	}
	if err != nil {
		return message.CannotStartLine("加载配置文件失败（"+shownConfigPath+"）", err.Error()), &contract.CannotStart{}
	}
	rev, tm, dirty := buildInfo()
	ver := versionString(num, rev, tm, dirty)
	return message.HelpScreen(name, ver, console.Width(), helpEntries(f)), nil
}

// helpRequested 认「要不要看帮助」。认法照 pflag 的语义收窄到本工具：从左往右扫，
// 带值选项吃掉下一个 token（所以 `-c "-h"` 不会被当成要看帮助）；不认识的选项当场停
// （老版那时就报错，轮不到帮助）；`--` 之后不再有选项；位置参数不拦（选项与位置参数
// 允许混排，老版同此）。带 `=` 的取值（`-c=x`）按不认识的选项停—— outcomes 与老版一致：
// 都不出帮助。
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
