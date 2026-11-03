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
	extras   []string // 位置参数：一段都不丢，全部收下交给闸门
	// extraOwner 第一个多余段紧跟在哪个选项的值后面（回溯 argv 得来），认不出为空。
	//
	// 它是事实，不是结论：「这算不算错、该怎么报」归闸门。收它是因为「值里有空格没加引号」
	// 与「参数打多了」在屏幕上要说成两句不同的话，而分辨它们要原始 argv——那东西只在
	// 解析站手上。
	extraOwner string
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
			return s.withExtraOwner(argv)
		case !strings.HasPrefix(tok, "-") || tok == "-":
			s.extras = append(s.extras, tok) // 位置参数：收下，判归闸门
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
	return s.withExtraOwner(argv)
}

// withExtraOwner 回溯第一个多余段，看它是不是紧跟在「某个选项的值」后面。
//
// 判据逐字照老版 args.go 的 splitOwner：多余段前面是一个值、值前面是一个以 `-` 开头的
// 选项，才认得出主。差别只在索引——老版传的是 os.Args[1:]（不含程序名），这里传整条 argv，
// 所以门槛从 2 抬到 3，argv[0] 是程序名，不能被当成「值前面的选项」。
func (s scanned) withExtraOwner(argv []string) scanned {
	if len(s.extras) == 0 {
		return s
	}
	first := s.extras[0]
	pos := -1
	for i, tok := range argv {
		if tok == first {
			pos = i
			break
		}
	}
	if pos < 3 {
		return s
	}
	value, flag := argv[pos-1], argv[pos-2]
	if flag != "--" && strings.HasPrefix(flag, "-") && !strings.HasPrefix(value, "-") {
		s.extraOwner = flag
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
			// 下发行的材料：`[env]` 原样带上，拼装归解析站。
			// 拷一份，不与配置共用同一张表——纸往下走，谁都不该改到配置。
			//
			// 解释器不在这里定：只有命令与脚本两种模式有它，见下面那一处（随模式定）。
			Env: copyEnv(cfg.Env),

			// 代填与改密认触发词的口径：执行站不读配置，只能从纸上来。
			MatchRegex:         cfg.interactive.Regex,
			MatchCaseSensitive: cfg.interactive.CaseSensitive,
			English:            cfg.locale.English,
		},
		Problems: s.problems,
	}

	vals, given := s.vals, s.given
	numberRaw, timeoutRaw, connectTimeoutRaw := vals["--number"], vals["--timeout"], vals["--connect-timeout"]

	// 多余的位置参数：一段都不许静默丢掉——静默丢掉会造成「看起来成功、实际少传」。
	//
	// 记事实，不判要不要拒。排在词法那两条之后、数值取值之前，对位老版：pflag 先报解析错，
	// 再 `fs.Args()` 查多余段，然后才轮到取值与合规。
	if len(s.extras) > 0 {
		p.Problems = append(p.Problems, contract.Problem{
			Kind:   contract.ProblemExtraArgs,
			Option: s.extraOwner,
			Raw:    strings.Join(s.extras, " "),
		})
	}

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
	// 「给没给」与「用不用」是两条事实：改密那两条互斥问的是前者（配置默认是 root 时，
	// 没给开关也不该报错）。
	p.Run.SudoGiven = sudoGiven || noSudoGiven
	if sudoGiven && noSudoGiven {
		p.Problems = append(p.Problems, contract.Problem{
			Kind:   contract.ProblemFlagConflict,
			Option: contract.FlagPairSudo,
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
	// -p 与 -d 是远端路径：两端空白在这里就去掉（老版也在 Parse 里先 filter 过一次）。
	// 纸上不留第二个形态——闸门拿到的就是要去判的那个值。
	p.Task.Path = strings.TrimSpace(vals["--path"])
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
		p.Task.Download = &contract.DownloadTask{RemotePath: strings.TrimSpace(vals["--download"])}
	case contract.ModePasswd:
		p.Task.Passwd = &contract.PasswdTask{Raw: vals["--change-password"]}
	}

	// 解释器（建站 05 的 04 号单）：命令模式取配置里的 command；脚本模式按后缀查表，
	// 没命中回退并记下那个后缀（闸门据此问一句）；其余三支为空。
	//
	// 上传／下载／改密的中间命令另有固定形态，配置随意改会把那些写死的命令弄失效，
	// 所以那三支一格都不填。
	switch mode {
	case contract.ModeCommand:
		p.Run.Interpreter = cfg.interpreter.Command
	case contract.ModeScript:
		p.Run.Interpreter, p.Run.FallbackExt = interpreterFor(p.Task.Script.Path, cfg.interpreter.Command, cfg.interpreter.Script)
	}

	// 材料自检（-s／-u／-d／-f）：读不到、不能当文本用，都属于「起不来」。
	//
	// 只在「这次干什么」已经定死、且纸上一个问题都没有时才做。有问题的纸一律先交给闸门：
	// 老版 pflag 解析完、CheckArguments 判完，才轮到读清单与 stat 材料（main.go 的步骤 5
	// 在读清单之前）。这一条守住那个次序——否则「-p 不能搭配 -c」会被「清单读不到」顶掉，
	// 使用者看到的是别的一件事。
	proceed := len(modes) == 1 && len(p.Problems) == 0
	if proceed {
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

	// 读清单：第一张纸的「对谁做」那一格。只在模式已定死、且没有多余参数时读——
	// 给了多个模式时先让闸门报「互斥」，有多余段时先让它报「多余的参数」，与材料自检
	// 同一条次序（老版就是这个顺序）。
	if proceed {
		m, nfail := readManifest(p.Resources.Manifest)
		if nfail != nil {
			return nil, nfail
		}
		if m.headerDropped {
			p.Notices = append(p.Notices, contract.Notice{Kind: contract.NoticeManifestHeader})
		}
		nodes, notices, nfail := buildNodes(m.rows, cfg, given["--key"])
		if nfail != nil {
			return nil, nfail
		}
		p.Nodes = nodes
		p.Notices = append(p.Notices, notices...)

		// 改密值（建站 05 的 07 号单）：按加密开关解释成明文，形态不对当场拦下。
		if p.Task.Passwd != nil {
			if fail := passwdValue(p, cfg.credentialCfg.Encrypt); fail != nil {
				return nil, fail
			}
		}

		// 上传源严格采集（建站 05 的 06 号单）：确认屏之前清点一遍。排在读清单之后，
		// 与老版同一个次序（清单读不通先报清单）。
		if p.Task.Upload != nil {
			if fail := collect(p, cfg); fail != nil {
				return nil, fail
			}
		}
	}

	// 代填表（建站 05 的 05 号单）：`-a` 的值读成一行一条的表，装进命令／脚本载荷。
	//
	// 只在两种模式里读：`-a` 与上传、下载互斥，那两种搭配先由闸门报「互斥」——
	// 在解析站抢先报「代填文件读不到」会把真正要说的那句话顶掉。
	if p.Run.Answer != "" && (p.Task.Command != nil || p.Task.Script != nil) {
		answers, afail := answerTable(p.Run.Answer, p.Run.MatchRegex, p.Run.MatchCaseSensitive)
		if afail != nil {
			return nil, afail
		}
		if p.Task.Command != nil {
			p.Task.Command.Answers = answers
		} else {
			p.Task.Script.Answers = answers
		}
	}

	// 下发行（建站 05 的 02 号单）：拼装只这一处，拼完装进纸上那两格。
	// 放在最后——脚本正文要先读出来、解释器要先定下来，两者都在前面。
	sealDownlink(p)
	return p, nil
}

// copyEnv 把 `[env]` 拷进纸里那张表。
//
// 为什么不留同一个 map：纸要往执行站递，递过去的是「这次要导入哪些变量」这份事实，
// 不是配置对象的一半。共用一张表等于让下游能改到配置，也等于两张纸之间互相串味。
func copyEnv(env map[string]string) map[string]string {
	if len(env) == 0 {
		return nil
	}
	out := make(map[string]string, len(env))
	for k, v := range env {
		out[k] = v
	}
	return out
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
