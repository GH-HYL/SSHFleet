package ssh

import (
	"strings"
)

// 语言变量前缀：收进登录 shell 内部，export 确保对内部所有命令及子进程生效
// （C.UTF-8 是 POSIX 标准，所有 Linux 发行版内置支持）。
const envPrefix = "export LC_ALL=en_US.UTF-8 LANG=en_US.UTF-8;"

// 执行模式（只用到这三种取值；由调用方从命令行参数解读后传入）。
const (
	modeSudo = "sudo"
)

// BuildCommand 构建下发的命令与 stdin 内容（spec 实现途径 1）：
// 命令/脚本内容经 session.Stdin 直喂，命令行只剩 `bash -lc '<env; sudo bash|bash>'`，
// 不再走 base64 通道——命令行不再有长度上限风险，也不需要引号转义套娃。
//
// 只收命令构建真正需要的几个值，不收整份命令行参数——本包是传输层，
// 不该认识参数载体（否则想单独测它得先凑齐一整套参数）。
//
// 参数：
//   - command:     命令模式下的命令原文（脚本模式传空）
//   - scriptBody:  脚本模式下的脚本内容（命令模式传空）
//   - interpreter: 脚本解释器（"bash" / "python3"；命令模式传空）
//   - noBash:      --nobash：命令模式专用，原样下发
//   - asRoot:      -m sudo：以 root 身份执行
//
// 返回 (下发命令, stdin 内容)：stdin 为空表示不喂输入（--nobash 命令模式）。
func BuildCommand(command, scriptBody, interpreter string, noBash, asRoot bool) (string, string) {
	// --nobash 为命令模式专用：原样下发，不套登录 shell、不喂 stdin
	if noBash && command != "" {
		return command, ""
	}

	sudo := ""
	if asRoot {
		sudo = modeSudo + " "
	}

	if scriptBody != "" {
		inner := envPrefix + " " + sudo + interpreter
		return "bash -lc " + shellQuote(inner), scriptBody
	}

	// 命令模式：stdin 喂命令文本，由 shell 读取执行
	inner := envPrefix + " " + sudo + "bash"
	return "bash -lc " + shellQuote(inner), command
}

// shellQuote 单引号包裹（对位旧 shlex.quote 的单引号路径）：内部单引号按 '\” 转义，
// 保证命令作为 bash -lc 的单个参数传递时不发生二次解析。
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
