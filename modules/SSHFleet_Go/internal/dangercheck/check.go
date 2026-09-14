// Package dangercheck 承载主干第 5 步的危险命令检测。
//
// 定位是操作助手——命中就提示风险等级，提醒使用者注意，**不是**拦截恶意命令的防御闸门。
// 解析层用 mvdan.cc/sh/v3/syntax（spec D25），判定层（包装命令剥离 / rm 旗标归一 /
// 间接执行递归 / 风险排序）自己实现；规则文件 TOML，语义与正则一字不动（D11）。
package dangercheck

import (
	"fmt"
	"os"

	"sshfleet/internal/cli"
)

// Check 读取待检内容（命令模式取 -c，脚本模式取脚本文件）并分析。
// 纯分析：不做打印、不做交互、不退出（处置由 main 按 Report 决定）。
func Check(a *cli.Args, rules *Rules) (*Report, error) {
	if a.Script == "" && a.Command == "" {
		return &Report{}, nil
	}

	report := &Report{IsScript: a.Script != ""}
	src := a.Command
	if a.Script != "" {
		data, err := os.ReadFile(a.Script)
		if err != nil {
			return nil, fmt.Errorf("读取脚本内容失败：%s\n原因：%v", a.Script, err)
		}
		report.ScriptPath = a.Script
		src = string(data)
	}

	report.Matches = Analyze(src, rules)
	return report, nil
}
