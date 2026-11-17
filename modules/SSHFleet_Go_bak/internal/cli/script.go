package cli

// 脚本模式的「这次要发什么」在这里单点决定：正文怎么整备。
// 原先散在两处各拼一遍（量下发行长度 / 真正执行）——各拼一份就会算出不同的长度，
// ADR-0010 的「量到的字节 = 发出去的字节」正落在这条上。
//
// 解释器不由这里选：它随参数一起解析（见 interpreter.go），存进 Args.Interpreter。
// 文件仍由各阶段各读：材料是「怎么拼」，读取是「何时读」，两者分开。

import (
	"sshfleet/internal/ssh"
)

// ScriptMaterial 脚本模式下发的材料。
type ScriptMaterial struct {
	Body string // 正文：剥 UTF-8 BOM、去掉整块首尾空白（ssh.ScriptBodyOf）
}

// ScriptMaterialOf 由参数与脚本文件内容拼出材料。
func ScriptMaterialOf(a *Args, data []byte) ScriptMaterial {
	return ScriptMaterial{
		Body: ssh.ScriptBodyOf(data),
	}
}
