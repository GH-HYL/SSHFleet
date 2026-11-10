package parse

import (
	"path/filepath"

	"sshfleet2/internal/console"
	"sshfleet2/internal/contract"
	"sshfleet2/internal/message"
)

// 解析站对外的唯一口子（出口白名单：至多一个函数，ADR-0037 决定三）。
//
// 它递回三样里的某一样：一屏话（帮助）、一张纸（进流水线）、一个「起不来」。
// 三样的组合就是塔尖认的结局，塔尖不自己判「这次该干嘛」。

// Run 吃整条 argv 与那行版本常量，吐四种结局里的某一种：一屏话（帮助）、一张纸（进流水线）、
// 一条小命令（交给密钥包）、是不是起不来。塔尖认这四样，自己不判断「这次该干嘛」。
//
// 配置、判据表、终端宽度都由本站自己取——塔尖除了 argv 和号，什么都不递
// （ADR-0032 第五条：它只搬，不看配置、不看文件在不在）。
func Run(argv []string, version string) (message.Line, *contract.Plan, *contract.KeyRun, *contract.CannotStart) {
	cfg, err := load()
	if err != nil {
		// 配置这一步失败时日志还没建起来（老版步骤 1 在步骤 2 之前），所以那一屏不带「日志：」行。
		return message.Line{}, nil, nil, cannotStartReasonBeforeLog("config", contract.CannotStartReason{
			Kind:   contract.CannotStartConfigLoad,
			Place:  shownConfigPath,
			Origin: err.Error(),
		})
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
		return screen, nil, nil, nil
	}

	plan, krun, fail := buildPlan(cfg, s)
	if fail != nil {
		return message.Line{}, nil, nil, fail
	}
	if krun != nil {
		// 一条小命令：不封运行上下文、不建纸——它不是流水线那一路（ADR-0032 第五条）。
		return message.Line{}, nil, krun, nil
	}
	// 运行上下文：把这次的原值封进纸上那一格。封不起来就是内部写死的密钥或格式坏了，
	// 按「起不来」报——不静默降级成明文，那等于把这一格的意义抹掉。
	sealed, reason, bad := sealRunMeta(runMetaBodyOf(plan, argv, cfg))
	if bad {
		return message.Line{}, nil, nil, cannotStartReasonBeforeLog("cli", reason)
	}
	plan.RunMeta = sealed
	return message.Line{}, plan, nil, nil
}

// cannotStartReasonBeforeLog 是配置那一步的「起不来」：老版读配置时日志还没建起来，
// 所以那一屏不带「日志：」行。原因按「类别 ＋ 参数」交上去，句子由 message 按类别成形
// （ADR-0044）。
//
// 到这里为止，「往纸里塞一句拼好的话」的三个入口（`cannotStart`、`cannotStartBeforeLog`、
// `cannotStartScreen`）已经全数删掉——纸里再没有地方放成品句子。
func cannotStartReasonBeforeLog(where string, reason contract.CannotStartReason) *contract.CannotStart {
	return &contract.CannotStart{Where: where, NoLog: true, Reason: reason}
}

// cannotStartReason 排一屏「起不来」：原因按类别交上去，句子由 message 成形（ADR-0044）。
func cannotStartReason(where string, reason contract.CannotStartReason) *contract.CannotStart {
	return &contract.CannotStart{Where: where, Reason: reason}
}
