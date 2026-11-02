package parse

import (
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"sshfleet2/internal/archive"
	"sshfleet2/internal/contract"
	"sshfleet2/internal/message"
)

// 认命令行：20 条选项、取值、默认值、认出五种模式。
//
// 语法错（不认识的选项、缺值、值认不出）**只记事实、不下结论**——判「这条不过」是闸门的活
// （上一张图 01 号工单：停只一处）。

// scanned 是「命令行读一遍」得到的全部事实。
//
// 只读一遍：帮助、取值、认不出的选项、位置参数都在同一次里记下。两套扫法会给出两个口径
// （`--command=ls -h` 这种带等号的写法，老版 pflag 认得出、另写一套扫法认不出），
// 这正是 16 号单 H9 指的那件事。
type scanned struct {
	vals     map[string]string
	given    map[string]bool
	problems []contract.Problem
	help     bool     // 要看帮助；扫到它就停（老版 pflag 当场返回 ErrHelp）
	extras   []string // 位置参数：老版一律报「多余的参数」，本阶段只记（见 16 号单）
}

// scan 从左往右扫一遍 argv。
//
// 停下有三处，与老版 pflag 一致：撞上 `-h`／`--help`（看帮助）、认不出的选项（当场报错）、
// 带值选项缺值（当场报错）。所以 `--zzz -h` 报的是选项错，`-h --zzz` 出的是帮助。
func scan(argv []string) scanned {
	s := scanned{vals: map[string]string{}, given: map[string]bool{}}
	if len(argv) <= 1 {
		s.help = true // 一个参数都不给：老版打帮助后以 0 退出
		return s
	}
	for i := 1; i < len(argv); i++ {
		tok := argv[i]
		switch {
		case tok == "--":
			s.extras = append(s.extras, argv[i+1:]...)
			return s
		case !strings.HasPrefix(tok, "-") || tok == "-":
			s.extras = append(s.extras, tok) // 位置参数：记下，不拦
			continue
		}

		name, inline, hasInline := splitOption(tok)
		if name == "-h" || name == "--help" {
			s.help = true
			return s
		}
		o, ok := lookupOption(name)
		if !ok {
			s.problems = append(s.problems, contract.Problem{Kind: contract.ProblemUnknownOption, Option: name})
			return s
		}
		if !o.takesValue {
			s.given[o.long] = true
			continue
		}
		val := inline
		if !hasInline {
			if i+1 >= len(argv) {
				s.problems = append(s.problems, contract.Problem{Kind: contract.ProblemMissingValue, Option: name})
				return s
			}
			i++
			val = argv[i]
		}
		s.vals[o.long] = val
		s.given[o.long] = true
	}
	return s
}

// buildPlan 把扫出来的事实认成一张纸。认不了的照样记在纸上的 Problems 里。
func buildPlan(cfg *settings, s scanned) (*contract.Plan, *contract.CannotStart) {
	p := &contract.Plan{
		Run: contract.RunParams{
			TimeoutConnect:  cfg.Execution.TimeoutConnect,
			TimeoutExecute:  cfg.Execution.TimeoutExecute,
			TimeoutTransfer: cfg.Execution.TimeoutTransfer,
			Sudo:            cfg.Execution.Sudo,
		},
		Problems: s.problems,
	}

	vals, given := s.vals, s.given
	numberRaw, timeoutRaw, connectTimeoutRaw := vals["--number"], vals["--timeout"], vals["--connect-timeout"]

	// 三条密钥命令本阶段不接（随阶段 08）：先按「还没接上」报，别让它落进「没指定模式」。
	for _, k := range []string{"--gen-key", "--key-status", "--convert-secret"} {
		if given[k] {
			return nil, cannotStart("cli", "", message.NotWiredKeyTools())
		}
	}

	// 三个数值选项：空值与 0 都算「不填」（老版 checkPositiveInt 的第一条），
	// 认不出或不是正数才记一条事实。
	if n, ok := positiveValue(numberRaw); ok {
		if numberRaw != "" {
			p.Run.Concurrency = n
		}
	} else {
		p.Problems = append(p.Problems, contract.Problem{
			Kind: contract.ProblemBadValue, Option: "-n", Raw: numberRaw, Detail: message.SubjectConcurrency,
		})
	}
	if n, ok := positiveValue(timeoutRaw); ok {
		// 没给就不动：这两个值有配置默认，写 0 会把默认抹掉（老版也是「不给就留原值」）。
		if timeoutRaw != "" {
			p.Run.TimeoutExecute = n
			p.Run.TimeoutTransfer = n
		}
	} else {
		p.Problems = append(p.Problems, contract.Problem{
			Kind: contract.ProblemBadValue, Option: "-t", Raw: timeoutRaw, Detail: message.SubjectTimeout,
		})
	}
	if n, ok := positiveValue(connectTimeoutRaw); ok {
		if connectTimeoutRaw != "" {
			p.Run.TimeoutConnect = n
		}
	} else {
		p.Problems = append(p.Problems, contract.Problem{
			Kind: contract.ProblemBadValue, Option: "-T", Raw: connectTimeoutRaw, Detail: message.SubjectConnect,
		})
	}

	p.Run.NoShell = given["--no-shell"]
	p.Run.Yes = given["--yes"]
	p.Run.Key = given["--key"]
	p.Run.Answer = vals["--answer"]
	p.Run.Remark = vals["--remark"]
	// 执行身份：纸上只留一个结论——「这一次到底用不用 sudo」。命令行给了哪个开关就按哪个算，
	// 都没给留着配置里的值。
	//
	// 「两个都给」不装进纸（一个布尔装不下两个事实），改成由解析站记一条 Problem：命令行
	// 原样在这儿看得见，闸门照它拒。使用者看到的仍是一句话的拒绝，不会被静默覆盖。
	sudoGiven, noSudoGiven := given["--sudo"], given["--no-sudo"]
	if sudoGiven && noSudoGiven {
		p.Problems = append(p.Problems, contract.Problem{
			Kind:   contract.ProblemFlagConflict,
			Option: "--sudo --no-sudo",
		})
	}
	if sudoGiven {
		p.Run.Sudo = true
	}
	if noSudoGiven {
		p.Run.Sudo = false
	}

	// 认出五种模式：给的这些原样装进任务格，零个／多个都只记事实，由闸门下结论。
	// -p 单独一格——命令与脚本模式下它不属于任何一种任务，却仍要判「不能搭配」。
	mode, modes := modeOf(vals)
	p.Mode = mode
	p.Task.Path = vals["--path"]
	if len(modes) > 1 {
		p.Problems = append(p.Problems, contract.Problem{
			Kind:   contract.ProblemMultiMode,
			Detail: strings.Join(modes, "、"),
		})
	}
	switch mode {
	case contract.ModeCommand:
		p.Task.Command = &contract.CommandTask{Text: vals["--command"]}
	case contract.ModeScript:
		p.Task.Script = &contract.ScriptTask{Path: vals["--script"]}
	case contract.ModeUpload:
		p.Task.Upload = &contract.UploadTask{Source: vals["--upload"]}
	case contract.ModeDownload:
		p.Task.Download = &contract.DownloadTask{RemotePath: vals["--download"]}
	case contract.ModePasswd:
		p.Task.Passwd = &contract.PasswdTask{Raw: vals["--change-password"]}
	}

	// 材料自检（-s／-u／-d／-f）：读不到、不能当文本用，都属于「起不来」。
	//
	// 只在「这次干什么」已经定死时才做：给了多个模式时先让闸门报「互斥」——老版就是这个
	// 顺序（先判互斥，再去 stat 文件），两份同时出错时使用者先看到的是「只能选一个」。
	if len(modes) == 1 {
		if fail := checkMaterial(p, given, vals); fail != nil {
			return nil, fail
		}
	}

	// 执行资源：这次用到的原料在哪。清单这一格是 `-f` 的原值，两种形态（文件路径、
	// 内联记录）都照原样装着；脚本与代填随阶段 05 加。
	p.Resources.Manifest = vals["--csv-file"]

	// 备注：没给 -r 就自动生成（命令取首词，脚本与上传取文件名，下载按远端规则切），
	// 生成过之后照样过平台过滤——它要当归档目录名的一段，也要原样显示在确认屏上。
	if p.Run.Remark == "" {
		p.Run.Remark = defaultRemark(p)
	}

	// 判据表：解析读一次、校验一次，随纸交给闸门。
	tables, fail := loadTables(mode)
	if fail != nil {
		return nil, fail
	}
	p.Tables = tables

	// 读清单：第一张纸的「对谁做」那一格。只在模式已定死时读——给了多个模式时
	// 先让闸门报「互斥」，与材料自检同一条次序（老版就是这个顺序）。
	if len(modes) == 1 {
		m, nfail := readManifest(p.Resources.Manifest)
		if nfail != nil {
			return nil, nfail
		}
		if m.notice.S != "" {
			p.Notices = append(p.Notices, m.notice)
		}
		nodes, notices, nfail := buildNodes(m.rows, cfg, given["--key"])
		if nfail != nil {
			return nil, nfail
		}
		p.Nodes = nodes
		p.Notices = append(p.Notices, notices...)
	}
	return p, nil
}

// modeOf 认出给了哪一种模式，以及一共给了几种。
//
// **按值认，不按「出现过没有」认**：`-c ""` 在老版里等于没给（Args.Command 是空串），
// 结局是「模式零个」。零个与多个都不在这里报错——那两句原话由闸门说（停只一处）。
func modeOf(vals map[string]string) (contract.Mode, []string) {
	var modes []string
	if vals["--command"] != "" {
		modes = append(modes, "--command")
	}
	if vals["--script"] != "" {
		modes = append(modes, "--script")
	}
	if vals["--upload"] != "" {
		modes = append(modes, "--upload")
	}
	if vals["--download"] != "" {
		modes = append(modes, "--download")
	}
	if vals["--change-password"] != "" {
		modes = append(modes, "--change-password")
	}
	if len(modes) != 1 {
		return contract.ModeNone, modes
	}
	switch modes[0] {
	case "--command":
		return contract.ModeCommand, modes
	case "--script":
		return contract.ModeScript, modes
	case "--upload":
		return contract.ModeUpload, modes
	case "--download":
		return contract.ModeDownload, modes
	}
	return contract.ModePasswd, modes
}

// positiveValue 解一个「正整数」选项：空值与 0 都算不填（老版 checkPositiveInt 第一条），
// 认得出就交回值，认不出交回 false。
func positiveValue(raw string) (int, bool) {
	if raw == "" || raw == "0" {
		return 0, true
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return 0, false
	}
	return n, true
}

// defaultRemark 没给 -r 时自动生成一次备注。
//
// 与老版同一条路：命令取首词、脚本与上传取本机文件名、下载取远端文件名（按 POSIX 规则切，
// 因为远端那条路径里 `\` 只是一个普通字符）。生成之后过一遍平台过滤——归档目录名要用它。
//
// 不截断：老版曾截到 8 字符，确认屏把备注原样显示出来之后出现了「afile.tx」这种半截名字，
// 已取消（2026-10 定）。
func defaultRemark(p *contract.Plan) string {
	switch {
	case p.Task.Command != nil:
		if fields := strings.Fields(p.Task.Command.Text); len(fields) > 0 {
			return archive.SanitizeRemark(fields[0])
		}
	case p.Task.Script != nil:
		return archive.SanitizeRemark(filepath.Base(p.Task.Script.Path))
	case p.Task.Upload != nil:
		return archive.SanitizeRemark(filepath.Base(p.Task.Upload.Source))
	case p.Task.Download != nil:
		return archive.SanitizeRemark(path.Base(p.Task.Download.RemotePath))
	}
	return ""
}

// splitOption 拆 `-c=x` 与 `--command=x`：返回值里带「是不是内联给的」。
func splitOption(tok string) (name, value string, inline bool) {
	if i := strings.Index(tok, "="); i > 0 {
		return tok[:i], tok[i+1:], true
	}
	return tok, "", false
}

// lookupOption 按短名或长名查选项表。
func lookupOption(name string) (option, bool) {
	for _, o := range options {
		if name == o.short || name == o.long {
			return o, true
		}
	}
	return option{}, false
}
