// 塔尖：只做四件——把控制台摆正、建日志、分叉走哪条路、给退出码。
//
// 它不判参数、不读配置、不拼句子：argv 与号递给解析站，纸递给闸门，话交给终端。
// 判据见 ADR-0032 第五条。
package main

import (
	"os"
	"path/filepath"
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
	fatal := func(where string, body message.FatalBody, logPath string) {
		lg.Fatal(where, body.Text())
		console.Print(message.Fatal(where, body, logPath))
		finish(exitcode.CannotStart)
	}

	if err != nil {
		console.Print(message.LogInitFailed(err))
		finish(exitcode.CannotStart)
	}
	lg.Startup(toolDir, toolFile)

	// 步骤 3 起：解析站给结局，闸门判放不放行。
	//
	// 解析站另交一条**延后的**「起不来」（材料读不到、判据表不齐）：那件事要等闸门判完前面
	// 几层才报得出口，所以它不进纸、也不在这一站用，先接着（ADR-0052 决定五）。
	say, plan, krun, fail, later := parse.Run(os.Args, appVersion)
	switch {
	case fail != nil:
		logPath := toolPath
		if fail.NoLog {
			logPath = "" // 配置那一步日志还没建起来，老版那一屏也不带「日志：」行
		}
		fatal(fail.Where, message.CannotStartBody(*fail), logPath)

	case krun != nil:
		// 第四种结局：一条不进流水线的小命令。塔尖只把命令交给密钥包、按它交回的结局收场，
		// 不判断「该不该跑」——那三条命令自己读自己那一小块配置（ADR-0032 第五条）。
		lg.Info(message.KeyModeEntered(krun.Which).Plain())
		if cannot := keytool.Run(*krun, credential.DefaultKeySource()); cannot != nil {
			fatal(cannot.Where, message.CannotStartBody(*cannot), toolPath)
		}

	case plan != nil:
		// 开工前那一问：加密开着才问主密钥齐不齐。不归拒绝——问完照跑（ADR-0034 决定六）。
		// 排在清单提示之前：老版这一步在清单读出来之前。
		if facts, need := credential.Precheck(plan.Run.Encrypted); need {
			console.Print(message.KeyPrecheckNote(facts))
			lg.Warn(message.KeyPrecheckTrace(facts.State).Plain())
		}

		// 解析站路上攒下的提示不在这里打：它们随纸递到闸门，由确认段开头印——被拒绝的那次、
		// 或者这一趟根本不出屏的那次，屏幕上不该先冒出「已移除表头」（ADR-0052 决定五）。
		refusal, note := gate.Run(plan, later != nil)
		if note.Text() != "" {
			lg.Warn(note.Text()) // 非交互放行危险命令的留痕：闸门不持有日志，交上来由这里落盘
		}
		if refusal != nil {
			console.Print(refusal.Say())
			// 拒绝也要落日志：老版这一族（参数认不出、搭配不合规、取值不成）全走 fatal，
			// 屏幕末尾报了「日志：<路径>」，日志里就得有那一行。闸门只交正文、不持日志句柄，
			// 环节标签与多行续行由日志包按老版规矩补（`[cli] …` / `[cli]   …`）。
			// 空正文表示这一拒不留痕——使用者自己答否、危险框上按「不继续」，都不是失败，
			// 老版在 ErrCancelled 那条路上也是直接退（审计 a3）。
			if trace := refusal.Trace(); trace.Plain() != "" {
				lg.Fatal(contract.WhereCLI, trace.Plain())
			}
			finish(exitcode.Refused)
		}

		// 放行但带延后项：这一趟注定要退，报它、按「起不来」收场。排在建归档目录之前——
		// 注定不成的那次不该留下产物。塔尖仍不判断，只认「有没有那一条」。
		if later != nil {
			fatal(later.Where, message.CannotStartBody(*later), toolPath)
		}

		// 交棒：从这一刻起信号只取消、不再收场。位置在建归档目录之前——所以
		// 「确认屏上按 Ctrl+C」归启动段，连归档目录都不建（ADR-0042）。
		intr.Handoff()

		// 步骤 8 前半：建本次的归档目录。本次执行的产物全在它下面，四条落点由它发。
		paths, aerr := archive.Create(contract.HistoryDir, string(plan.Mode), plan.Run.Remark, time.Now())
		if aerr != nil {
			// 站的原文原样上屏：那一句中文现在还在站里（`archive` 一处），迁走之后这一类别就没人用
			// 了（ADR-0047）。塔尖不替它加壳，也不许把 error 直接交给屏幕。
			fatal(contract.WhereOutput, message.CannotStartBody(contract.CannotStart{
				Reason: contract.CannotStartReason{Kind: contract.CannotStartSystemFault, Origin: aerr.Error()},
			}), toolPath)
		}
		// 塔尖自己做的动作，自己留痕（老版 `:435`／`:443`）。各站的「解析成功、合规通过、
		// 读取节点成功」那一族不补：句柄只在塔尖，补它们就得替站宣布成功（见 `message.TraceArchiveCreated`）。
		lg.Success(message.TraceArchiveCreated(paths.Dir).Plain())
		lg.Info(message.TraceExecLogRotated(paths.ExecLog).Plain())

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
			fatal(cannot.Where, message.CannotStartBody(*cannot), toolPath)
		}
		// 执行期日志写完、切回工具日志（老版 `:484`）。老版排在「执行结束：成功 N 台」那条
		// 统计行**之前**，所以这一句也放在结算与执行日志收尾之前。
		lg.Info(message.TraceExecLogClosed(paths.ExecLog).Plain())

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
			// 同上：`report` 那四句中文还在站里，这一屏只把原文接过来（ADR-0047）。
			fatal(contract.WhereOutput, message.CannotStartBody(contract.CannotStart{
				Reason: contract.CannotStartReason{Kind: contract.CannotStartSystemFault, Origin: rerr.Error()},
			}), toolPath)
		}
		// 报告站已经把这几句打上屏了；这里只管留痕——它不持日志句柄。
		for _, w := range warns {
			lg.Warn(w.Plain())
		}
		// 收尾那一句（老版 `:510`）：只在真跑完执行链之后写，帮助／拒绝／起不来那几条路都不写。
		// 路径照老版用相对形态（`history/SSHFleetTools.log`）——日志是给人看的，工作目录就在旁边。
		lg.Info(message.TraceExit(filepath.Join(toolDir, toolFile)).Plain())
		// 再补一条与开头同形的分隔线：一次运行在日志里自成一段（画法归日志包）。
		lg.Closing()

	default:
		console.Print(say)
		finish(exitcode.Success)
	}

	// 跑完：有节点失败也退 0（既有行为，`exitcode` 那四格不动）。
	finish(exitcode.Success)
}
