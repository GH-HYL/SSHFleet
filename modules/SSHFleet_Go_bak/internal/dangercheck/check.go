// Package dangercheck 承载主干第 5 步的危险命令检测。
//
// 定位是操作助手——命中就提示风险等级，提醒使用者注意，**不是**拦截恶意命令的防御闸门。
// 判定对象是命令**原文**：不论命令写在 bash、python、perl、node、php 里，都按同一份文本
// 匹配（ADR-0019）。判定层自己实现：去注释、路径片段整备、逐条规则匹配、风险排序；
// 规则文件 TOML，谁能算危险全在规则文件里，代码里没有命令名单。
package dangercheck

import (
	"fmt"
	"os"

	"sshfleet/internal/common"
)

// Check 读取待检内容（命令模式取 -c，脚本模式取脚本文件）并分析。
// 纯分析：不做打印、不做交互、不退出（处置由 main 按 Report 决定）。
func Check(command, scriptPath string, rules *Rules) (*Report, error) {
	if scriptPath == "" && command == "" {
		return &Report{}, nil
	}

	report := &Report{IsScript: scriptPath != ""}
	src := command
	if scriptPath != "" {
		data, err := os.ReadFile(scriptPath)
		if err != nil {
			return nil, common.ErrUserReason(fmt.Sprintf("读取脚本内容失败：%s", scriptPath), err.Error())
		}
		report.ScriptPath = scriptPath
		src = string(data)
	}

	report.Matches = Analyze(src, rules)
	return report, nil
}
