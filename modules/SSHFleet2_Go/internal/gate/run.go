package gate

import (
	"strings"

	"sshfleet2/internal/archive"
	"sshfleet2/internal/ask"
	"sshfleet2/internal/contract"
	"sshfleet2/internal/mask"
	"sshfleet2/internal/message"
)

// 闸门的唯一口子：递一张纸，收「一个拒绝」（nil 表示放行）。
//
// 它只管裁量：参数搭配、互斥、取值、危险命令、使用者当场喊停。它不读文件、不读配置、
// 不读判据表——纸上前一站给什么它就判什么。它也不打屏、不拼句子：每条判据只交事实
// （哪一条没过、哪个选项、哪段原文），句子由 message 造，屏由塔尖或 ask 打出去
// （ADR-0034 决定八、ADR-0028 决定一）。

// Run 判这张纸放不放行。
//
// 第二个返回值是「留给日志的一句话」：非交互（--yes）跳过危险命令确认时要留痕，而日志句柄
// 在塔尖手里（闸门不持有它），所以闸门把这个事实交上去，由塔尖落盘。
//
// 判的先后照老版 CheckArguments：解析层的错先报，模式其次，逐条搭配检查按老版顺序来，
// 三个数值选项放在最后（老版把 -n／-t／-T 的取值检查放在整段末尾）。同一份命令里错两处时，
// 使用者先看到哪一句由这个顺序定。
func Run(p *contract.Plan) (*Refusal, message.Line) {
	if r := parseRefusal(p); r != nil {
		return r, message.Line{}
	}
	if r := modeRefusal(p); r != nil {
		return r, message.Line{}
	}
	if r := argumentRefusal(p); r != nil {
		return r, message.Line{}
	}
	if r := valueRefusal(p); r != nil {
		return r, message.Line{}
	}
	if r, note := dangerRefusal(p); r != nil || note.Text() != "" {
		return r, note
	}
	// 信封合法性排在最后一道：前面几条判的都是「这次给的东西对不对」，
	// 这一条判的是「纸本身有没有被造错」——它不该抢在人的毛病前面报。
	if r := envelopeRefusal(p); r != nil {
		return r, message.Line{}
	}
	return confirmRefusal(p)
}

// envelopeRefusal 验信封：标签说是哪一种活，载荷就得是那一份、而且只有那一份。
//
// 这条只在纸被造错时才响（写着命令、装的却是脚本），正常使用撞不上——所以它的话说得直白，
// 不往使用者身上推。算「输入不合规」那一类，不另立一类（ADR-0035 决定七）。
func envelopeRefusal(p *contract.Plan) *Refusal {
	set := make([]string, 0, 5)
	for _, one := range []struct {
		name    string
		present bool
	}{
		{"命令", p.Task.Command != nil},
		{"脚本", p.Task.Script != nil},
		{"上传", p.Task.Upload != nil},
		{"下载", p.Task.Download != nil},
		{"改密", p.Task.Passwd != nil},
	} {
		if one.present {
			set = append(set, one.name)
		}
	}
	if len(set) > 1 {
		return refuse(message.RefuseEnvelope("同时装了" + strings.Join(set, "、")))
	}

	want := ""
	switch p.Mode {
	case contract.ModeCommand:
		want = "命令"
	case contract.ModeScript:
		want = "脚本"
	case contract.ModeUpload:
		want = "上传"
	case contract.ModeDownload:
		want = "下载"
	case contract.ModePasswd:
		want = "改密"
	default:
		return nil // 模式零个／多个由上一条判据报，这里不抢
	}
	if len(set) == 0 {
		return refuse(message.RefuseEnvelope("标签是" + want + "，载荷却是空的"))
	}
	if set[0] != want {
		return refuse(message.RefuseEnvelope("标签是" + want + "，装的是" + set[0]))
	}
	return nil
}

// confirmRefusal 是闸门的最后一道：把这次要做什么摊给使用者看，问一句。
//
// 屏与问句由 message 造，打屏与收行归 ask——闸门自己一行都不写（ADR-0032 第四条）。
// 答否算当场喊停；读到输入结束算「没答完」，同样不放行。
func confirmRefusal(p *contract.Plan) (*Refusal, message.Line) {
	if p.Run.Yes {
		// --yes 是替人回答了「y」，不是替人看了屏：老版此时不打参数屏，只打这一行。
		ask.Ask(message.ConfirmSkippedLine(), true, true)
		return nil, message.ConfirmSkippedTrace()
	}
	switch ask.AskWith(message.ConfirmScreen(p, mask.InlineRecordIf(p.Resources.Manifest)), message.ExecConfirmPrompt(), true) {
	case ask.NoInput:
		// 屏幕上「输入结束，操作已取消」已经由 ask 打过，这里不再叠一句。
		return &Refusal{Say: message.Line{}}, message.Line{}
	case ask.No:
		return &Refusal{Say: message.CancelLine()}, message.CancelTrace()
	}
	return nil, message.Line{}
}

// refuse 把一句正文交给终端：走 stderr，老版那副 [ERROR] 外壳，末尾附日志路径。
func refuse(body string) *Refusal {
	return &Refusal{Say: message.Fatal("cli", body, archive.ToolLogPath(contract.HistoryDir))}
}

// parseRefusal 解析层那三条：认不出的选项、带值选项缺值、多出来的位置参数。
//
// 三条都排在合规之前，对位老版：pflag 先报解析错，`fs.Args()` 再查多余段，
// 然后才进 CheckArguments。
func parseRefusal(p *contract.Plan) *Refusal {
	for _, pr := range p.Problems {
		switch pr.Kind {
		case contract.ProblemUnknownOption:
			return refuse(message.RefuseUnknownOption(pr.Option))
		case contract.ProblemMissingValue:
			return refuse(message.RefuseMissingValue(pr.Option))
		case contract.ProblemExtraArgs:
			return refuse(message.RefuseExtraArgs(pr.Option, pr.Raw))
		}
	}
	return nil
}

// modeRefusal 模式零个或多个。多个先判：零个与多个到纸面时是互斥的，先判哪个不影响结论。
func modeRefusal(p *contract.Plan) *Refusal {
	for _, pr := range p.Problems {
		if pr.Kind == contract.ProblemMultiMode {
			return refuse(message.RefuseModeMultiple())
		}
	}
	if p.Mode == contract.ModeNone {
		return refuse(message.RefuseModeNone())
	}
	return nil
}

// argumentRefusal 逐条搭配检查，顺序逐字照老版 CheckArguments。
func argumentRefusal(p *contract.Plan) *Refusal {
	command := p.Task.Command
	script := p.Task.Script
	upload := p.Task.Upload
	download := p.Task.Download
	passwd := p.Task.Passwd
	path := p.Task.Path

	// --no-shell 只能和 -c 一起用（上传下载不拼下发命令，没有外壳可剥）。
	if p.Run.NoShell && (script != nil || upload != nil || download != nil) {
		return refuse(message.RefuseNoShellWithOtherMode())
	}

	// 改密与三个批量参数互斥：老版把这一段放在进任何模式分支之前。
	if passwd != nil {
		switch {
		case p.Run.Answer != "":
			return refuse(message.RefusePasswdWithAnswer())
		case path != "":
			return refuse(message.RefusePasswdWithPath())
		case p.Run.Key:
			return refuse(message.RefusePasswdWithKey())
		}
	}

	// -p：上传时是服务器上的目录（判远端形态、要以 / 收尾）；命令与脚本用不到它。
	if path != "" {
		if upload != nil {
			if r := checkRemotePath("-p", "上传目录", path, true); r != nil {
				return r
			}
		} else if command != nil || script != nil {
			return refuse(message.RefusePathWithCommand())
		}
	}

	// -d：必须配 -p，且 -d 恒为远端路径（本机目录读不读得到归解析的材料自检）。
	if download != nil {
		if path == "" {
			return refuse(message.RefuseDownloadNeedsPath())
		}
		if r := checkRemotePath("-d", "远程路径", download.RemotePath, false); r != nil {
			return r
		}
	}

	// -u：必须配 -p（源文件读不读得到归解析的材料自检）。
	if upload != nil && path == "" {
		return refuse(message.RefuseUploadNeedsPath())
	}

	// 执行身份：一对开关同时写就是自相矛盾。事实由解析站记（纸上只有一个结论字段，
	// 装不下「两个都给」），这里只照那条事实下结论。
	if flagConflict(p, "--sudo --no-sudo") {
		return refuse(message.RefuseSudoConflict())
	}

	// -c 给了但只有空白（给空串算没给，落进「模式零个」那一条）。
	if command != nil && strings.TrimSpace(command.Text) == "" {
		return refuse(message.RefuseCommandEmpty())
	}

	// 改密的新密码只有空白（本阶段只判形态；解密随阶段 05）。
	if passwd != nil && strings.TrimSpace(passwd.Raw) == "" {
		return refuse(message.RefusePasswdEmpty())
	}
	return nil
}

// flagConflict 报「这一对开关同时给了」。事实在解析站记着（那里看得见命令行原样），
// 闸门只认这条事实、自己不再比较选项——纸上没有那两格可比。
func flagConflict(p *contract.Plan, options string) bool {
	for _, pr := range p.Problems {
		if pr.Kind == contract.ProblemFlagConflict && pr.Option == options {
			return true
		}
	}
	return false
}

// checkRemotePath 判一个远端路径的形态：先从 / 写起、不含回车换行；上传的目录还要求以 / 收尾。
// 上传与下载共用这一份，`what` 只决定那句话怎么称呼这个位置。
func checkRemotePath(flag, what, value string, needDirSlash bool) *Refusal {
	path := strings.TrimSpace(value)
	if !strings.HasPrefix(path, "/") {
		return refuse(message.RefuseRemotePathNotAbsolute(flag, what, path))
	}
	if strings.ContainsAny(path, "\r\n") {
		return refuse(message.RefuseRemotePathHasNewline(flag, what, path))
	}
	if needDirSlash && !strings.HasSuffix(path, "/") {
		return refuse(message.RefuseUploadDirNeedsSlash(path))
	}
	return nil
}

// valueRefusal 三个数值选项的取值：老版放在整段检查的最后。
func valueRefusal(p *contract.Plan) *Refusal {
	for _, pr := range p.Problems {
		if pr.Kind == contract.ProblemBadValue {
			return refuse(message.RefuseBadPositiveInt(pr.Option, pr.Detail, pr.Raw))
		}
	}
	return nil
}

// dangerRefusal 危险命令：禁止档直接拒，提醒档问人，答否算当场喊停。
func dangerRefusal(p *contract.Plan) (*Refusal, message.Line) {
	source, isScript, scriptPath := dangerSource(p)
	if strings.TrimSpace(source) == "" {
		return nil, message.Line{}
	}
	report := analyze(source, p.Tables.Danger)
	report.IsScript = isScript
	report.ScriptPath = scriptPath
	if len(report.Matches) == 0 {
		return nil, message.Line{}
	}

	if report.Matches[0].Level == contract.RiskForbidden {
		// 屏幕给框、日志留一行——老版两处都写，跳过哪一处都是改行为。
		return &Refusal{Say: message.DangerBox(report, true)}, message.TraceForbidden(report.Matches[0].Content)
	}

	answer := ask.Ask(message.DangerBox(report, false), p.Run.Yes, false)
	switch answer {
	case ask.NoInput:
		// 屏幕上「输入结束，操作已取消」已经由 ask 打过，这里不再叠一句。
		return &Refusal{Say: message.Line{}}, message.Line{}
	case ask.No:
		return &Refusal{Say: message.CancelLine()}, message.Line{}
	}
	if p.Run.Yes {
		// 非交互跳过确认要留痕：老版由交互器顺手写，本版把这个事实交回塔尖，由塔尖落日志。
		return nil, message.TraceDanger(report.Matches[0])
	}
	return nil, message.Line{}
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
