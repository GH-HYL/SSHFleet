package cli

// 解释器只在命令模式（-c）与脚本模式（-s）里出现：前者用配置里的 command，后者按脚本
// 后缀查 script 映射、没命中回退到 command。上传 / 下载 / 改密不参与——它们各自的中间
// 命令另有固定形态（见 batch / ssh 侧），配置随意改会把那些写死的命令弄失效。
//
// 回退为什么要问一句：后缀五花八门，用户没配到的（.bin 之类）也得能跑，但"没配却照样
// 按某个解释器跑"这件事要让用户知情、点头，免得跑错了还不知道（2026-10-07 作者定）。

import (
	"fmt"
	"path"
	"strings"

	"sshfleet/internal/common"
	"sshfleet/internal/config"
	"sshfleet/internal/ssh"
)

// configShown 面向用户展示的配置文件路径（与 main 的 configPathShown 同形：不带 ./）。
const configShown = "config/SSHFleet.conf"

// interpreterOf 解析本次执行用的解释器：
//   - 脚本模式（-s）：按文件后缀查映射，命中用它；没命中回退到命令解释器
//   - 命令模式（-c）：用命令解释器
//   - 其余模式：不涉及解释器，返回空串
//
// 在 Parse 里调用一次，结果存进 Args.Interpreter——执行侧一律从 Args 取，不回头读配置。
func interpreterOf(a *Args, cfg *config.Config) string {
	switch {
	case a.Script != "":
		if v, ok := MatchScriptInterpreter(a.Script, cfg.Interpreter.Script); ok {
			return v
		}
		return cfg.Interpreter.Command
	case a.Command != "":
		return cfg.Interpreter.Command
	default:
		return ""
	}
}

// MatchScriptInterpreter 按脚本文件名后缀查映射：命中返回 (解释器, true)。
// 只取文件名的最后一段扩展名，比较忽略大小写（配置里的后缀键在加载时已归一为小写）。
func MatchScriptInterpreter(scriptPath string, table map[string]string) (string, bool) {
	ext := strings.ToLower(path.Ext(scriptPath))
	if ext == "" {
		return "", false
	}
	v, ok := table[ext]
	return v, ok
}

// ConfirmInterpreterFallback 脚本后缀没配到解释器时，当场问一句是否用回退解释器继续；
// 用户不答应（或读到 EOF）返回 ErrCancelled，由 main 统一退出。
// 非交互（--yes）下 Confirm 直接返回已确认，即"默认为 yes"（2026-10-07 作者定）。
func ConfirmInterpreterFallback(a *Args, cfg *config.Config, in *common.Interactor) error {
	if a.Script == "" {
		return nil
	}
	if _, ok := MatchScriptInterpreter(a.Script, cfg.Interpreter.Script); ok {
		return nil
	}
	ext := path.Ext(a.Script)
	ok, err := in.Confirm(fmt.Sprintf(
		"脚本后缀 %q 没有配置解释器\n"+
			"  想换别的解释器：在 %s 的 [interpreter.script] 里加一行，如 %q = \"解释器名\"\n"+
			"  继续用 %s 作为解释器吗？",
		ext, configShown, ext, cfg.Interpreter.Command), false)
	if err != nil {
		return err
	}
	if !ok {
		in.Notice(fmt.Sprintf("已取消：脚本后缀 %q 没有配置解释器，本次执行终止\n", ext))
		return common.ErrCancelled
	}
	return nil
}

// IsBashInterpreter 解释器是不是 bash（归一后认名，`/bin/bash`、`bash5` 都算）。
// 用于参数屏解释器行的展示门槛：配置就是默认的 bash 时不占行、不提示。
func IsBashInterpreter(interpreter string) bool {
	return ssh.InterpreterBaseName(interpreter) == "bash"
}

// checkAnswerInterpreter 拦下 -a 不支持的解释器。
//
// -a 的正文要作"一段程序文本"交给解释器，而各家的参数不一样（shell / python 是 -c，
// perl / ruby / node / lua 是 -e，php 是 -r，见 ssh.ProgramTextArg）。表外的解释器没有统一
// 入口：配进来再用 -a 会静默不跑（perl 用 -c 甚至"只做语法检查、跑完就退"）。与其让它闷着
// 出错，不如在参数合规检查阶段当场说清（2026-10-07 作者定）。
func checkAnswerInterpreter(a *Args) error {
	if a.Answer == "" {
		return nil // 没用 -a，与解释器无关
	}
	if _, ok := ssh.ProgramTextArg(a.Interpreter); ok {
		return nil
	}
	return fmt.Errorf(
		"-a 代填不支持解释器 %q\n"+
			"目前支持：%s\n"+
			"提示：请改 %s 的 [interpreter]，换成上面这些；或去掉 -a 后重跑",
		a.Interpreter,
		strings.Join(ssh.SupportedInterpreters(), "、"),
		configShown)
}
