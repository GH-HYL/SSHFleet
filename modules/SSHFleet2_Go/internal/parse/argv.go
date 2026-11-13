package parse

import (
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"sshfleet2/internal/archive"
	"sshfleet2/internal/contract"
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
// buildPlan 把扫出来的事实认成一张纸。认不了的照样记在纸上的 Problems 里。
//
// 第三个返回值是「延后的起不来」：判据表不齐、材料读不到，这两类都要等到闸门判完前面
// 几层才说得出口（ADR-0052 决定五）。它不进纸，与纸一起交给塔尖。
func buildPlan(cfg *settings, s scanned) (*contract.Plan, *contract.KeyRun, *contract.CannotStart) {
	// 三条密钥命令先认一次：它们是独立功能，不进流水线（开发者 2026-11-06 裁定）。
	// 词法有问题时不进这一支——未知选项、缺值仍归闸门拒，与 0x 定的先后一致。
	if krun, fail := keyRunOf(s); fail != nil || krun != nil {
		return nil, krun, fail
	}

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
			Encrypted:          cfg.credentialCfg.Encrypt,
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

	// 三个数值选项：空值与 0 都算「不填」（老版 checkPositiveInt 的第一条），
	// 认不出或不是正数才记一条事实。
	if n, ok := positiveValue(numberRaw); ok {
		if numberRaw != "" {
			p.Run.Concurrency = n
		}
	} else {
		p.Problems = append(p.Problems, contract.Problem{
			Kind: contract.ProblemBadValue, Option: "-n", Raw: numberRaw, Subject: contract.SubjectConcurrency,
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
			Kind: contract.ProblemBadValue, Option: "-t", Raw: timeoutRaw, Subject: contract.SubjectTimeout,
		})
	}
	if n, ok := positiveValue(connectTimeoutRaw); ok {
		if connectTimeoutRaw != "" {
			p.Run.TimeoutConnect = n
		}
	} else {
		p.Problems = append(p.Problems, contract.Problem{
			Kind: contract.ProblemBadValue, Option: "-T", Raw: connectTimeoutRaw, Subject: contract.SubjectConnect,
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
		// 哪几个模式一起给，闸门不看：零个与多个到纸面时已经互斥，它只认这条事实。
		p.Problems = append(p.Problems, contract.Problem{Kind: contract.ProblemMultiMode})
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

	// 执行资源：这次用到的原料在哪。清单这一格是 `-f` 的原值，两种形态（文件路径、
	// 内联记录）都照原样装着；脚本与代填随阶段 05 加。
	p.Resources.Manifest = vals["--csv-file"]

	// 判据表：解析读一次、校验一次，随纸交给闸门。
	//
	// 第四层要用它（判决），所以读不成的这件事要排在一至三层之后才说得出口——失败记成
	// 一条延后项，纸照样交上去；闸门拿到的是零值的表，危险检测不会命中，塔尖报这一条。
	tables, tableFail := loadTables(mode)
	p.Tables = tables

	// 第五、六层：脚本、上传源、下载目录、清单、节点、改密值、代填表。
	//
	// 只在「这次干什么」已经定死、且纸上一个问题都没有时才读。有问题的纸一律先交给闸门：
	// 「-p 不能搭配 -c」不该被「清单读不到」顶掉。失败同样不当场报，另交一条延后项。
	proceed := len(modes) == 1 && len(p.Problems) == 0
	var materialFail *contract.CannotStart
	if proceed {
		materialFail = materials(p, cfg, given, vals)
	}

	// 备注：没给 -r 就自动生成（命令取首词，脚本与上传取文件名，下载按远端规则切），
	// 生成过之后照样过平台过滤——它要当归档目录名的一段，也要原样显示在确认屏上。
	if p.Run.Remark == "" {
		p.Run.Remark = defaultRemark(p)
	}

	// 下发行（建站 05 的 02 号单）：拼装只这一处，拼完装进纸上那两格。
	// 放在最后——脚本正文要先读出来、解释器要先定下来，两者都在前面。
	sealDownlink(p)

	// 延后项只交一条，按层序取最早的那个：判据表（第四层）先于材料（第五、六层）。
	deferred := tableFail
	if deferred == nil {
		deferred = materialFail
	}
	return p, nil, deferred
}

// materials 按第五、六层的次序读这一趟的材料，交回第一个失败。
//
// 次序照 ADR-0052 决定二那张表：脚本 → 上传源 → 下载目录 → 清单（这一层证「拿得到」），
// 再是清单的语法与逐行、改密值、代填表（下一层证「内容对」）。层内这个先后不按使用者
// 敲写的次序，也不按老版的实现顺序。
//
// 交回失败只交一条：再往后读也没有意义，而且塔尖一次只报一句。
func materials(p *contract.Plan, cfg *settings, given map[string]bool, vals map[string]string) *contract.CannotStart {
	if fail := materialScript(p, vals); fail != nil {
		return fail
	}
	if p.Task.Upload != nil {
		if fail := collect(p, cfg); fail != nil {
			return fail
		}
	}
	if fail := materialDownloadDir(vals); fail != nil {
		return fail
	}
	if fail := materialManifestFile(vals); fail != nil {
		return fail
	}

	// 缺 `-f` 不在这里报：那是第三层的「必填」，由闸门说一句自己的话（ADR-0052 决定二）。
	// 本站只在给了清单时读它。
	if p.Resources.Manifest != "" {
		m, fail := readManifest(p.Resources.Manifest)
		if fail != nil {
			return fail
		}
		if m.headerDropped {
			p.Notices = append(p.Notices, contract.Notice{Kind: contract.NoticeManifestHeader})
		}
		nodes, notices, fail := buildNodes(m.rows, cfg, given["--key"])
		if fail != nil {
			return fail
		}
		p.Nodes = nodes
		p.Notices = append(p.Notices, notices...)
	}

	// 改密值（建站 05 的 07 号单）：按加密开关解释成明文，形态不对当场拦下。
	if p.Task.Passwd != nil {
		if fail := passwdValue(p, cfg.credentialCfg.Encrypt); fail != nil {
			return fail
		}
	}

	// 上传并发建议：按总大小与配置阈值算，再按节点数封顶——所以它排在读清单之后。
	applyUploadAdvice(p, cfg)

	// 代填表（建站 05 的 05 号单）：`-a` 的值读成一行一条的表，装进命令／脚本载荷。
	//
	// 只在两种模式里读：`-a` 与上传、下载互斥，那两种搭配先由闸门报「互斥」——
	// 在解析站抢先报「代填文件读不到」会把真正要说的那句话顶掉。
	if p.Run.Answer != "" && (p.Task.Command != nil || p.Task.Script != nil) {
		answers, afail := answerTable(p.Run.Answer, p.Run.MatchRegex, p.Run.MatchCaseSensitive)
		if afail != nil {
			return afail
		}
		if p.Task.Command != nil {
			p.Task.Command.Answers = answers
		} else {
			p.Task.Script.Answers = answers
		}
	}
	return nil
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

// keyRunOf 认出「这次是不是一条密钥命令」。
//
// 两条纪律：
//   - 词法有问题时不进这一支（解析站只把问题记在纸上，判不过归闸门——与 0x 定的先后一致）；
//   - 两条命令一起给、或与批量参数一起给，当场是「起不来」，不走闸门。三条命令是独立功能，
//     不掺和流水线那套合规判定（开发者 2026-11-06 裁定）。
func keyRunOf(s scanned) (*contract.KeyRun, *contract.CannotStart) {
	if len(s.problems) > 0 {
		return nil, nil
	}
	tools := keyToolsOf(s)
	if len(tools) == 0 {
		return nil, nil
	}
	// 多余的位置参数先拦：老版在解析命令行那一步就报「出现多余的参数」，比密钥命令那两条
	// 互斥更早。这一支只在密钥命令上生效——批量执行那条路仍把问题记在纸上交闸门（0x 定的先后）。
	if len(s.extras) > 0 {
		return nil, cannotStartReason("cli", contract.CannotStartReason{Kind: contract.CannotStartExtraArgs, Value: s.extraOwner, Origin: strings.Join(s.extras, " ")})
	}
	if len(tools) > 1 {
		return nil, cliReason(contract.CannotStartReason{
			Kind:  contract.CannotStartKeyToolAlone,
			Value: strings.Join(tools, "、"),
		})
	}
	if given := batchParamsOf(s); len(given) > 0 {
		return nil, cliReason(contract.CannotStartReason{
			Kind:  contract.CannotStartKeyToolWithBatch,
			Place: tools[0],
			Value: strings.Join(given, "、"),
		})
	}
	which := contract.KeyToolGen
	switch tools[0] {
	case "--key-status":
		which = contract.KeyToolStatus
	case "--convert-secret":
		which = contract.KeyToolConvert
	}
	return &contract.KeyRun{Which: which, Path: s.vals["--convert-secret"], Yes: s.given["--yes"]}, nil
}

// keyToolsOf 是这次给了哪几条密钥命令（用使用者看到的名字）。
//
// `--convert-secret` 按**值**认，不按「出现过没有」认：老版 `a.ConvertSecret != ""`，
// 给一个空串等于没给。
func keyToolsOf(s scanned) []string {
	var names []string
	if s.given["--gen-key"] {
		names = append(names, "--gen-key")
	}
	if s.given["--key-status"] {
		names = append(names, "--key-status")
	}
	if s.vals["--convert-secret"] != "" {
		names = append(names, "--convert-secret")
	}
	return names
}

// batchParamsOf 是这次给了哪些批量执行参数（用使用者看到的名字）。
//
// 顺序与老版 `batchParamsGiven` 逐项相同：值类按原值非空认，开关类按给没给认。
// `--yes` 不算批量参数：它对密钥命令也有意义（跳过覆盖确认），且不指向任何节点。
// batchParamLongs 是「不能和密钥命令一起给」的那批参数，按报错里出现的次序排。
//
// 名字不另抄一串，按长名回选项表查（新增一个选项只改选项表那一处）。
var batchParamLongs = []string{
	"--command", "--script", "--upload", "--download", "--change-password",
	"--csv-file", "--path", "--number", "--remark", "--timeout", "--connect-timeout",
}

// flagParamLongs 是那四个只看「给没给」的开关。
var flagParamLongs = []string{"--key", "--no-shell", "--sudo", "--no-sudo"}

// paramName 是报错里怎么写这个参数：有短名写短名，没有就写长名（与帮助里的写法同源）。
func paramName(long string) string {
	if o, ok := lookupOption(long); ok && o.short != "" {
		return o.short
	}
	return long
}

func batchParamsOf(s scanned) []string {
	// 老版在解析命令行那一步就把「没给 -r 时的自动备注」填好了，所以那条检查会把它当成交了 -r。
	// 这里照同一笔账：四种模式任一给了值、而 -r 没给时，自动备注已经生成。
	autoRemark := s.vals["--remark"] == "" &&
		(s.vals["--command"] != "" || s.vals["--script"] != "" ||
			s.vals["--upload"] != "" || s.vals["--download"] != "")

	var given []string
	for _, long := range batchParamLongs {
		hit := s.vals[long] != ""
		if long == "--remark" {
			hit = hit || autoRemark
		}
		if hit {
			given = append(given, paramName(long))
		}
	}
	for _, long := range flagParamLongs {
		if s.given[long] {
			given = append(given, paramName(long))
		}
	}
	return given
}
