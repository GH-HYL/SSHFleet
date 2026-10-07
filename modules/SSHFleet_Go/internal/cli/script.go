package cli

// 脚本模式的「这次要发什么」在这里单点决定：正文怎么整备、解释器怎么选。
// 原先散在三处各拼一遍（量下发行长度 / 真正执行 / 日志说明）——各拼一份就会算出
// 不同的长度，ADR-0010 的「量到的字节 = 发出去的字节」正落在这条上。
//
// 文件仍由各阶段各读：材料是「怎么拼」，读取是「何时读」，两者分开。

import (
	"path"

	"sshfleet/internal/ssh"
)

// ScriptMaterial 脚本模式下发的材料。
type ScriptMaterial struct {
	Body        string // 正文：剥 UTF-8 BOM、去掉整块首尾空白（ssh.ScriptBodyOf）
	Interpreter string // 解释器：默认 bash，.py 用 python3
}

// ScriptInterpreter 脚本模式的解释器；非脚本模式（-s 为空）返回空串。
func ScriptInterpreter(a *Args) string {
	if a.Script == "" {
		return ""
	}
	if path.Ext(a.Script) == ".py" {
		return "python3"
	}
	return "bash"
}

// ScriptMaterialOf 由参数与脚本文件内容拼出材料。
func ScriptMaterialOf(a *Args, data []byte) ScriptMaterial {
	return ScriptMaterial{
		Body:        ssh.ScriptBodyOf(data),
		Interpreter: ScriptInterpreter(a),
	}
}
