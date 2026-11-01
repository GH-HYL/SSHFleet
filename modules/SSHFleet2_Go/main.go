// 塔尖：只做四件——把控制台摆正、建日志、分叉走哪条路、给退出码。
//
// 它不判参数、不读配置、不拼句子：argv 与号递给解析站，纸递给闸门，话交给终端。
// 判据见 ADR-0032 第五条。
package main

import (
	"os"

	"sshfleet2/internal/console"
	"sshfleet2/internal/contract"
	"sshfleet2/internal/exitcode"
	"sshfleet2/internal/gate"
	"sshfleet2/internal/log"
	"sshfleet2/internal/message"
	"sshfleet2/internal/parse"
)

// appVersion 是版本号的唯一出处：发布脚本按这一行改号（release.py 认这个字面量）。
// 初值取老工程当前发布号；两套并存期间脚本只改老工程那一行，这里不许自己编号。
const appVersion = "1.0.0"

func main() {
	// 步骤 0：适配控制台。Windows 出厂控制台是 936 码页，我们写出去的是 UTF-8 字节。
	// 退出前一定要还回去——os.Exit 不跑 defer，所以每一条退出路径自己收尾。
	console.Prepare()

	// 步骤 2：建工具日志。建不起来就是「起不来」，此时连日志都没有，那屏不带日志行。
	lg, err := log.Init(contract.HistoryDir, contract.ToolLogName)

	// finish 是唯一的退场收尾：关日志、还码页、查表退出。os.Exit 不跑 defer，每条路径都得显式收，
	// 所以只写这一处（lg 为 nil 时 Close 自己是空动作）。
	finish := func(kind exitcode.Kind) {
		_ = lg.Close()
		console.Restore()
		os.Exit(exitcode.Code(kind))
	}

	if err != nil {
		console.Print(message.LogInitFailed(err))
		finish(exitcode.CannotStart)
	}
	lg.Startup()

	// 步骤 3 起：解析站给结局，闸门判放不放行。
	say, plan, fail := parse.Run(os.Args, appVersion)
	switch {
	case fail != nil:
		lg.Fatal(fail.Where, fail.Body)
		console.Print(fail.Say)
		finish(exitcode.CannotStart)

	case plan != nil:
		refusal, note := gate.Run(plan)
		if note.S != "" {
			lg.Warn(note.S) // 非交互放行危险命令的留痕：闸门不持有日志，交上来由这里落盘
		}
		if refusal != nil {
			console.Print(refusal.Say)
			finish(exitcode.Refused)
		}
		// 过渡形态：放行之后没有下一站，随阶段 04 接上执行站。
		console.Print(message.NotWiredLine())
		finish(exitcode.Refused)

	default:
		console.Print(say)
		finish(exitcode.Success)
	}
}
