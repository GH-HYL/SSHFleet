package parse

import (
	"strconv"
	"strings"

	"sshfleet2/internal/contract"
	"sshfleet2/internal/message"
)

// 认命令行：20 条选项、取值、默认值、认出五种模式。
//
// 语法错（不认识的选项、缺值、值认不出）**只记事实、不下结论**——判「这条不过」是闸门的活
// （上一张图 01 号工单：停只一处）。

// buildPlan 把 argv 认成一张纸。认不了的当事实记在纸上的 Problems 里。
func buildPlan(cfg *settings, argv []string) (*contract.Plan, *contract.CannotStart) {
	p := &contract.Plan{
		Run: contract.RunParams{
			TimeoutConnect:  cfg.Execution.TimeoutConnect,
			TimeoutExecute:  cfg.Execution.TimeoutExecute,
			TimeoutTransfer: cfg.Execution.TimeoutTransfer,
			Sudo:            cfg.Execution.Sudo,
		},
	}

	vals := map[string]string{}
	given := map[string]bool{}
	numberRaw, timeoutRaw, connectTimeoutRaw := "", "", ""
	numberInvalid, timeoutInvalid, connectInvalid := false, false, false

	for i := 1; i < len(argv); i++ {
		tok := argv[i]
		switch {
		case tok == "--":
			i = len(argv) // 后面全是位置参数，本工具不认
			continue
		case !strings.HasPrefix(tok, "-") || tok == "-":
			continue // 位置参数不拦（老版允许与选项混排）
		}

		name, inline, hasInline := splitOption(tok)
		o, ok := lookupOption(name)
		if !ok {
			p.Problems = append(p.Problems, contract.Problem{Kind: contract.ProblemUnknownOption, Option: name})
			continue
		}
		if !o.takesValue {
			given[o.long] = true
			continue
		}
		val := inline
		if !hasInline {
			if i+1 >= len(argv) {
				p.Problems = append(p.Problems, contract.Problem{Kind: contract.ProblemMissingValue, Option: name})
				continue
			}
			i++
			val = argv[i]
		}
		vals[o.long] = val
		given[o.long] = true

		// 三个数值选项当场解一遍：认不出就是「值认不出」这条事实。
		switch o.long {
		case "--number":
			numberRaw = val
			if _, err := strconv.Atoi(val); err != nil {
				numberInvalid = true
			}
		case "--timeout":
			timeoutRaw = val
			if _, err := strconv.Atoi(val); err != nil {
				timeoutInvalid = true
			}
		case "--connect-timeout":
			connectTimeoutRaw = val
			if _, err := strconv.Atoi(val); err != nil {
				connectInvalid = true
			}
		}
	}

	// 三条密钥命令本阶段不接（随阶段 08）：先按「还没接上」报，别让它落进「没指定模式」。
	for _, k := range []string{"--gen-key", "--key-status", "--convert-secret"} {
		if given[k] {
			return nil, &contract.CannotStart{
				Where: "cli",
				Say:   message.Fatal("cli", "这条用法还没接上：三条密钥命令随阶段 08 接线", ""),
				Body:  "这条用法还没接上：三条密钥命令随阶段 08 接线",
			}
		}
	}

	// 数值选项：认不出记一条事实；认得出就落到执行参数上。
	if numberRaw != "" && !numberInvalid {
		if n, err := strconv.Atoi(numberRaw); err == nil && n > 0 {
			p.Run.Concurrency = n
		}
	}
	if timeoutRaw != "" && !timeoutInvalid {
		if n, err := strconv.Atoi(timeoutRaw); err == nil && n > 0 {
			p.Run.TimeoutExecute = n
			p.Run.TimeoutTransfer = n
		}
	}
	if connectTimeoutRaw != "" && !connectInvalid {
		if n, err := strconv.Atoi(connectTimeoutRaw); err == nil && n > 0 {
			p.Run.TimeoutConnect = n
		}
	}
	for _, bad := range []struct {
		flag    string
		invalid bool
		detail  string
	}{
		{"-n", numberInvalid, "并发数必须是正整数"},
		{"-t", timeoutInvalid, "命令或传输超时必须是正整数"},
		{"-T", connectInvalid, "连接超时必须是正整数"},
	} {
		if bad.invalid {
			p.Problems = append(p.Problems, contract.Problem{
				Kind: contract.ProblemBadValue, Option: bad.flag, Raw: vals[longOf(bad.flag)], Detail: bad.detail,
			})
		}
	}

	p.Run.NoShell = given["--no-shell"]
	p.Run.Yes = given["--yes"]
	p.Run.Key = given["--key"]
	p.Run.Remark = vals["--remark"]
	if given["--sudo"] {
		p.Run.Sudo = true
	}
	if given["--no-sudo"] {
		p.Run.Sudo = false
	}

	// 认出五种模式：给的这些原样装进任务格，零个／多个都只记事实，由闸门下结论。
	mode, modes := modeOf(given)
	p.Mode = mode
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
		p.Task.Upload = &contract.UploadTask{Source: vals["--upload"], RemoteDir: vals["--path"]}
	case contract.ModeDownload:
		p.Task.Download = &contract.DownloadTask{RemotePath: vals["--download"], LocalDir: vals["--path"]}
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

	// 判据表：解析读一次、校验一次，随纸交给闸门。
	tables, fail := loadTables(mode)
	if fail != nil {
		return nil, fail
	}
	p.Tables = tables
	return p, nil
}

// modeOf 认出给了哪一种模式，以及一共给了几种。
//
// 零个与多个都不在这里报错——那两句原话由闸门说（停只一处）。这里只如实回答「给了哪些」。
func modeOf(given map[string]bool) (contract.Mode, []string) {
	var modes []string
	if given["--command"] {
		modes = append(modes, "--command")
	}
	if given["--script"] {
		modes = append(modes, "--script")
	}
	if given["--upload"] {
		modes = append(modes, "--upload")
	}
	if given["--download"] {
		modes = append(modes, "--download")
	}
	if given["--change-password"] {
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

// longOf 短名对回长名，取值时用。
func longOf(short string) string {
	if o, ok := lookupOption(short); ok {
		return o.long
	}
	return short
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
