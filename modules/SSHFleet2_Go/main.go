// 塔尖：只做三件——分叉走哪条路、把信封交给下一站、给退出码。
//
// 判据见 ADR-0032 第五条。
package main

import (
	"os"

	"sshfleet2/internal/console"
	"sshfleet2/internal/parse"
)

// appVersion 是版本号的唯一出处：发布脚本按这一行改号（release.py 认这个字面量）。
// 初值取老工程当前发布号；两套并存期间脚本只改老工程那一行，这里不许自己编号。
// 日常改代码不动它——只有 tools/release/release.py 在发布时改。
const appVersion = "1.0.0"

func main() {
	// 塔尖不拼句子、不判「该不该看帮助」：argv 与号递给解析站，话交终端，按结局给退出码。
	// 退出码 1 是过渡值（起不来），阶段 03 换成退出码表的查表结果。
	say, fail := parse.Run(os.Args, appVersion)
	console.Print(say)
	if fail != nil {
		os.Exit(1)
	}
}
