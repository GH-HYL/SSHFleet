// 塔尖：只做四件——把控制台摆正、建日志、分叉走哪条路、给退出码。
//
// 它不判参数、不读配置、不拼句子：argv 与号递给解析站，纸递给闸门，话交给终端。
// 判据见 ADR-0032 第五条。
package main

import (
	"os"
	"time"

	"sshfleet2/internal/archive"
	"sshfleet2/internal/console"
	"sshfleet2/internal/contract"
	"sshfleet2/internal/credential"
	"sshfleet2/internal/execute"
	"sshfleet2/internal/exitcode"
	"sshfleet2/internal/gate"
	"sshfleet2/internal/interrupt"
	"sshfleet2/internal/keytool"
	"sshfleet2/internal/log"
	"sshfleet2/internal/message"
	"sshfleet2/internal/parse"
	"sshfleet2/internal/report"
	"sshfleet2/internal/settle"
)

// appVersion 是版本号的唯一出处：发布脚本按这一行改号（release.py 认这个字面量）。
// 初值取老工程当前发布号；两套并存期间脚本只改老工程那一行，这里不许自己编号。
const appVersion = "1.0.0"

func main() {
	// 步骤 0：适配控制台。Windows 出厂控制台是 936 码页，我们写出去的是 UTF-8 字节。
	// 退出前一定要还回去——os.Exit 不跑 defer，所以每一条退出路径自己收尾。
	console.Prepare()

	// 日志句柄先声明、后赋值：中断的登记要在「日志还没建起来」那一段就有人管，
	// 而那两个收场口（finish／fatal）都要关它。Close 对 nil 是空动作。
	var lg *log.Logger

	// finish 是唯一的退场收尾：关日志、还码页、查表退出。os.Exit 不跑 defer，每条路径都得显式收，
	// 所以只写这一处（lg 为 nil 时 Close 自己是空动作）。
	finish := func(kind exitcode.Kind) {
		_ = lg.Close()
		console.Restore()
		os.Exit(exitcode.Code(kind))
	}

	// 中断：控制台摆正之后立刻登记，覆盖解析、闸门、确认屏那一段——那一段最容易按，
	// 而且那时还没改过任何东西，收场就是「还码页、关日志、退 130」（ADR-0042）。
	// 登记一直活到进程结束，中途不撤；执行段的信号由同一条通道进来，只取消。
	intr := interrupt.Begin(func() { finish(exitcode.Interrupted) })

	// 步骤 2：建工具日志。建不起来就是「起不来」，此时连日志都没有，那屏不带日志行。
	// 两格落点由归档包发（本包不拼路径），本包只把这两格递给日志包。
	toolDir, toolFile := archive.ToolLog(contract.HistoryDir)
	toolPath := archive.ToolLogPath(contract.HistoryDir)
	lg, err := log.Init(toolDir, toolFile)

	// fatal 是「起不来」的唯一收尾：落日志、打那一屏、按种类退。
	// 那一屏向 message 要——纸上只装事实（哪一环、哪句话、带不带日志行），成品的话只在
	// message 里成形（契约里连字都装不了：`message.Line` 字段不导出）。
	fatal := func(where, body, logPath string) {
		lg.Fatal(where, body)
		console.Print(message.Fatal(where, body, logPath))
		finish(exitcode.CannotStart)
	}

	if err != nil {
		console.Print(message.LogInitFailed(err))
		finish(exitcode.CannotStart)
	}
	lg.Startup(toolDir, toolFile)

	// 步骤 3 起：解析站给结局，闸门判放不放行。
	say, plan, krun, fail := parse.Run(os.Args, appVersion)
	switch {
	case fail != nil:
		logPath := toolPath
		if fail.NoLog {
			logPath = "" // 配置那一步日志还没建起来，老版那一屏也不带「日志：」行
		}
		fatal(fail.Where, message.CannotStartText(*fail), logPath)

	case krun != nil:
		// 第四种结局：一条不进流水线的小命令。塔尖只把命令交给密钥包、按它交回的结局收场，
		// 不判断「该不该跑」——那三条命令自己读自己那一小块配置（ADR-0032 第五条）。
		lg.Info(message.KeyModeEntered(krun.Which).Plain())
		if cannot := keytool.Run(*krun, credential.DefaultKeySource()); cannot != nil {
			fatal(cannot.Where, message.CannotStartText(*cannot), toolPath)
		}

	case plan != nil:
		// 开工前那一问：加密开着才问主密钥齐不齐。不归拒绝——问完照跑（ADR-0034 决定六）。
		// 排在清单提示之前：老版这一步在清单读出来之前。
		if facts, need := credential.Precheck(plan.Run.Encrypted); need {
			console.Print(message.KeyPrecheckNote(facts))
			lg.Warn(message.KeyPrecheckTrace(facts.State).Plain())
		}

		// 解析站路上攒下的提示先打出去：老版的次序也是「读清单的提示在前、确认屏在后」。
		// 纸上是种类与事实，句子向 message 要。
		for _, n := range plan.Notices {
			console.Print(message.NoticeLine(n))
		}
		refusal, note := gate.Run(plan)
		if note.Text() != "" {
			lg.Warn(note.Text()) // 非交互放行危险命令的留痕：闸门不持有日志，交上来由这里落盘
		}
		if refusal != nil {
			console.Print(refusal.Say())
			finish(exitcode.Refused)
		}

		// 交棒：从这一刻起信号只取消、不再收场。位置在建归档目录之前——所以
		// 「确认屏上按 Ctrl+C」归启动段，连归档目录都不建（ADR-0042）。
		intr.Handoff()

		// 步骤 8 前半：建本次的归档目录。本次执行的产物全在它下面，四条落点由它发。
		paths, aerr := archive.Create(contract.HistoryDir, string(plan.Mode), plan.Run.Remark, time.Now())
		if aerr != nil {
			fatal(contract.WhereOutput, aerr.Error(), toolPath)
		}

		// 闸门交上来的留痕也要进归档执行日志（审计 09-1）：`--yes` 跳过确认、危险命令放行、
		// 脚本后缀回退这几件事原来只躺在工具日志里，归档打开来查不到。执行日志是追加打开，
		// 所以这一句先写、就落在最前面——它是「这一轮怎么开跑的」，排在节点明细之前。
		if note.Text() != "" {
			if execLog, lerr := log.InitExec(paths.ExecLog); lerr == nil {
				execLog.Warn(note.Text())
				_ = execLog.Close()
			}
		}

		// 步骤 8：执行站连上去跑，跑完交第二张纸。取消经上下文递下去——同一个上下文，
		// 启动段就建好了，执行站不必另造一个。
		paper, cannot := execute.Run(intr.Context(), plan, paths.Terminal, paths.ExecLog)
		if cannot != nil {
			fatal(cannot.Where, message.CannotStartText(*cannot), toolPath)
		}

		// 步骤 9、10：结算站算总账，报告站出屏幕统计块与 `report.txt`。
		ledger := settle.Run(paper)

		// 执行期日志收尾那一行：老版也是算完统计才补它，所以排在结算之后、报告之前。
		// 句子由 message 成形，本包只把第三张纸递过去、让日志包落盘。
		if execLog, lerr := log.InitExec(paths.ExecLog); lerr == nil {
			execLog.Info(message.ExecSummaryLog(ledger))
			_ = execLog.Close()
		}

		// 被中断那次要多两句，次序照老版：屏幕那句在前、日志那句在后，都排在统计块之前。
		// 「被中断」这件事读纸，不由本包自己看上下文——事实只有一个产出地（ADR-0042 决定二）。
		if paper.Interrupted {
			console.Print(message.InterruptLine())
			lg.Warn(message.InterruptLog().Text())
		}

		warns, rerr := report.Run(ledger, paths)
		if rerr != nil {
			fatal(contract.WhereOutput, rerr.Error(), toolPath)
		}
		// 报告站已经把这几句打上屏了；这里只管留痕——它不持日志句柄。
		for _, w := range warns {
			lg.Warn(w.Plain())
		}

	default:
		console.Print(say)
		finish(exitcode.Success)
	}

	// 跑完：有节点失败也退 0（既有行为，`exitcode` 那四格不动）。
	finish(exitcode.Success)
}
