package gate

import (
	"fmt"
	"strings"

	"sshfleet2/internal/ask"
	"sshfleet2/internal/contract"
	"sshfleet2/internal/log"
	"sshfleet2/internal/message"
)

// 闸门的唯一口子：递一张纸，收「一个拒绝」（nil 表示放行）。
//
// 它只管裁量：参数搭配、互斥、取值、危险命令、使用者当场喊停。它不读文件、不读配置、
// 不读判据表——纸上前一站给什么它就判什么。它也不打屏、不拼句子：话说由 message 造，
// 屏由塔尖或 ask 打出去。

// Run 判这张纸放不放行。
//
// 第二个返回值是「留给日志的一句话」：非交互（--yes）跳过危险命令确认时要留痕，而日志句柄
// 在塔尖手里（闸门不持有它），所以闸门把这个事实交上去，由塔尖落盘。
func Run(p *contract.Plan) (*Refusal, contract.Text) {
	if r := syntaxRefusal(p); r != nil {
		return r, contract.Text{}
	}
	if r := checkArguments(p); r != nil {
		return r, contract.Text{}
	}
	return dangerRefusal(p)
}

// refuseCLI 造一个「输入不合规」的拒绝：stderr，老版那副 [ERROR] 外壳。
func refuseCLI(head, body string) *Refusal {
	full := head
	if body != "" {
		full = head + "：" + strings.TrimRight(body, "\r\n")
	}
	return &Refusal{Say: message.Fatal("cli", full, log.ToolPath())}
}

// syntaxRefusal 先把解析时记下的事实判掉：认不出的选项、缺值、值认不出、多个模式。
//
// 老版这几条分别出自两个出口：「选项/参数解析失败」（词法层）与「选项/参数合规性检查未通过」
// （语义层）。这里按同一分工分成两句外壳。
func syntaxRefusal(p *contract.Plan) *Refusal {
	for _, pr := range p.Problems {
		switch pr.Kind {
		case contract.ProblemUnknownOption:
			return refuseCLI("选项/参数解析失败", fmt.Sprintf("unknown flag: %s", pr.Option))
		case contract.ProblemMissingValue:
			return refuseCLI("选项/参数解析失败", fmt.Sprintf("flag needs an argument: %s", pr.Option))
		case contract.ProblemBadValue:
			return refuseCLI("选项/参数合规性检查未通过",
				message.ProblemHint(fmt.Sprintf("%s 参数格式错误，%s，当前值：%s", pr.Option, pr.Detail, pr.Raw),
					"给一个正整数（单位见该选项的说明）"))
		case contract.ProblemMultiMode:
			return refuseCLI("选项/参数合规性检查未通过",
				"执行模式选项：-c、-s、-u、-d、--change-password 互斥，只能指定一个\n提示：一次只能做一件事，请只保留一个模式")
		}
	}
	return nil
}

// checkArguments 逐条照老版 CheckArguments（除去材料那一半）：
// 模式、搭配、取值、改密参数。
func checkArguments(p *contract.Plan) *Refusal {
	if p.Mode == contract.ModeNone {
		return refuseCLI("选项/参数合规性检查未通过", "执行模式选项：-c、-s、-u、-d、--change-password 互斥，只能指定一个\n"+
			"提示：你没有指定任何模式，请从下面五个里选一个：\n"+
			"      -c 执行命令   -s 执行脚本   -u 上传文件   -d 下载文件   --change-password 批量改密")
	}

	upload := p.Task.Upload
	download := p.Task.Download
	command := p.Task.Command
	script := p.Task.Script

	// --no-shell 只能和 -c 一起用。
	if p.Run.NoShell && (script != nil || upload != nil || download != nil) {
		return refuseCLI("选项/参数合规性检查未通过",
			"--no-shell 只能和 -c 一起使用\n提示：请去掉 --no-shell，或改用 -c 执行")
	}

	// -p：上传时是服务器上的目录（必须以 / 收尾），命令与脚本不需要它。
	if upload != nil && upload.RemoteDir != "" {
		dir := upload.RemoteDir
		if !strings.HasPrefix(dir, "/") {
			return refuseCLI("选项/参数合规性检查未通过",
				fmt.Sprintf("-p 参数指定的上传目录必须是绝对路径，当前值：%s\n提示：服务器上的路径要从 / 写起，例如 /opt/app/", dir))
		}
		if strings.ContainsAny(dir, "\r\n") {
			return refuseCLI("选项/参数合规性检查未通过",
				fmt.Sprintf("-p 参数指定的上传目录里有回车或换行：%s\n提示：一条命令只写一个路径，检查复制时是不是把换行带进来了", dir))
		}
		if !strings.HasSuffix(dir, "/") {
			return refuseCLI("选项/参数合规性检查未通过",
				fmt.Sprintf("上传模式：-p 必须是以 / 结尾的目录，当前值：%s\n提示：-p 只表示\"放到哪个目录\"，工具不会替你改文件名，所以要以 / 收尾", dir))
		}
	}
	if p.Task.Command != nil || p.Task.Script != nil {
		if dir := pathValue(p); dir != "" {
			return refuseCLI("选项/参数合规性检查未通过",
				"-p 参数不能搭配 -c 或 -s 使用\n提示：只有上传、下载才用 -p；命令与脚本模式不需要它")
		}
	}

	// -d 必须配 -p，且 -d 恒为远端路径。
	if download != nil {
		if download.LocalDir == "" {
			return refuseCLI("选项/参数合规性检查未通过",
				"-d 参数必须搭配 -p 参数使用\n提示：-p 写文件要存到本机的哪个目录，例如 -p ./logs/")
		}
		if fail := checkRemotePath("-d", "远程路径", download.RemotePath); fail != "" {
			return refuseCLI("选项/参数合规性检查未通过", fail)
		}
	}

	// -u 必须配 -p。
	if upload != nil && upload.RemoteDir == "" {
		return refuseCLI("选项/参数合规性检查未通过",
			"-u 参数必须搭配 -p 参数使用\n提示：-p 写文件要放到服务器上的哪个目录，例如 -p /opt/app/")
	}

	// -c 不能为空。
	if command != nil && strings.TrimSpace(command.Text) == "" {
		return refuseCLI("选项/参数合规性检查未通过",
			"-c 参数不能为空，请提供要执行的命令\n提示：-c 后面跟命令原文，例如 -c \"systemctl status sshd\"")
	}

	// 改密参数（本阶段只判形态；解密随阶段 05）。
	if p.Task.Passwd != nil && strings.TrimSpace(p.Task.Passwd.Raw) == "" {
		return refuseCLI("选项/参数合规性检查未通过",
			"--change-password 的新密码不能为空\n提示：后面跟这次要统一改成的新密码")
	}

	_ = script
	return nil
}

// pathValue 纸上的 -p 原值：上传与下载各存一处，命令与脚本模式只可能有下载那一处。
func pathValue(p *contract.Plan) string {
	if p.Task.Download != nil {
		return p.Task.Download.LocalDir
	}
	return ""
}

// checkRemotePath 远端路径的形态检查：必须绝对、不许含换行。返回空串表示通过。
func checkRemotePath(flag, what, p string) string {
	trimmed := strings.TrimSpace(p)
	if !strings.HasPrefix(trimmed, "/") {
		return fmt.Sprintf("%s 参数指定的%s必须是绝对路径，当前值：%s\n提示：服务器上的路径要从 / 写起，例如 /opt/app/", flag, what, trimmed)
	}
	if strings.ContainsAny(trimmed, "\r\n") {
		return fmt.Sprintf("%s 参数指定的%s里有回车或换行：%s\n提示：一条命令只写一个路径，检查复制时是不是把换行带进来了", flag, what, trimmed)
	}
	return ""
}

// dangerRefusal 危险命令：禁止档直接拒，提醒档问人，答否算当场喊停。
func dangerRefusal(p *contract.Plan) (*Refusal, contract.Text) {
	source, isScript, scriptPath := dangerSource(p)
	if strings.TrimSpace(source) == "" {
		return nil, contract.Text{}
	}
	report := analyze(source, p.Tables.Danger)
	report.IsScript = isScript
	report.ScriptPath = scriptPath
	if len(report.Matches) == 0 {
		return nil, contract.Text{}
	}

	if report.Matches[0].Level == contract.RiskForbidden {
		return &Refusal{Say: message.DangerBox(report, true)}, contract.Text{}
	}

	answer := ask.Ask(message.DangerBox(report, false), p.Run.Yes, false)
	switch answer {
	case ask.NoInput:
		// 屏幕上「输入结束，操作已取消」已经由 ask 打过，这里不再叠一句。
		return &Refusal{Say: contract.Text{}}, contract.Text{}
	case ask.No:
		return &Refusal{Say: message.CancelLine()}, contract.Text{}
	}
	if p.Run.Yes {
		// 非交互跳过确认要留痕：老版由交互器顺手写，本版把这个事实交回塔尖，由塔尖落日志。
		return nil, message.TraceDanger(report.Matches[0])
	}
	return nil, contract.Text{}
}

// dangerSource 取出要判的原文：命令模式取 -c，脚本模式取脚本正文。
func dangerSource(p *contract.Plan) (string, bool, string) {
	if p.Task.Command != nil {
		return p.Task.Command.Text, false, ""
	}
	if p.Task.Script != nil {
		return p.Task.Script.Body, true, p.Task.Script.Path
	}
	return "", false, ""
}
