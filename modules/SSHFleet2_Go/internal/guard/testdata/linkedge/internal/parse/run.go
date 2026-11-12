package parse

import _ "unsafe"

// 违规样本：用 linkname 直接绑别的包的内部函数——import 表上一条边都没有。
//
//go:linkname runCommand fixture/internal/execute.runCommand
func runCommand(cmd string) {}
