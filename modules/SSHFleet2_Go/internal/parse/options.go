package parse

import (
	"sshfleet2/internal/message"
)

// 选项表：帮助里「有哪些选项、各是什么」的唯一出处，与将来认命令行取值用的是同一张
//（分开就有两个来源，ADR-0032 第五条）。文字逐字照老版 cli.helpEntries 与 pflag 登记，
// 改一个字都是改使用者可见面，须先问。

// option 命令行上的一个开关：名字、长名、带不带值。帮助的显示行从这张表排。
type option struct {
	short, long string
	takesValue  bool
}

// options 二十条：十三条带值、七条开关。顺序即帮助显示顺序（老版同序）。
var options = []option{
	{"-c", "--command", true},
	{"-s", "--script", true},
	{"-u", "--upload", true},
	{"-d", "--download", true},
	{"-f", "--csv-file", true},
	{"-p", "--path", true},
	{"", "--sudo", false},
	{"", "--no-sudo", false},
	{"-t", "--timeout", true},
	{"-T", "--connect-timeout", true},
	{"-n", "--number", true},
	{"-r", "--remark", true},
	{"", "--no-shell", false},
	{"", "--yes", false},
	{"", "--change-password", true},
	{"-a", "--answer", true},
	{"-k", "--key", false},
	{"", "--gen-key", false},
	{"", "--key-status", false},
	{"", "--convert-secret", true},
}

// helpRows 是帮助的排布：分组、顺序与标记——哪个选项、哪个分组、什么顺序。
//
// 字不在这里：说明按长名向 `message.HelpDesc` 要，分组名是 `message.HelpGroup*` 那几个常量。
// 改一句口吻只翻 `message`（措辞迁移 04 号单）。
//
// **选项名也不在这里**——它是选项表的一部分，按长名从 `options` 里查回来。两边各写一串名字
// 迟早漂移（新增一个选项，一处改了另一处没改），这正是 16 号单 H8 指的那件事。
type helpRow struct {
	group string // 非空：一个分组标题（`message.HelpGroup*` 之一）
	blank bool   // 空行
	long  string // 选项长名，短名与说明都按它查回来
	tag   string // 可选：默认值或当前配置，留空不排那一列
}

// helpRowsOf 排一张表；标记里要用的配置值就地取，说明与分组名向 `message` 要。
func helpRowsOf(f execution) []helpRow {
	return []helpRow{
		{group: message.HelpGroupMode},
		{long: "--command"},
		{long: "--script"},
		{long: "--upload"},
		{long: "--download"},
		{long: "--change-password"},

		{blank: true},

		{group: message.HelpGroupTarget},
		{long: "--csv-file"},
		{long: "--path"},

		{blank: true},

		{group: message.HelpGroupExec},
		{long: "--number", tag: message.HelpTagDefaultAll()},
		{long: "--remark"},
		{long: "--sudo", tag: message.HelpTagCurrentConfig(f.Sudo)},
		{long: "--no-sudo"},
		{long: "--yes"},
		{long: "--answer"},
		{long: "--no-shell"},
		{long: "--timeout", tag: message.HelpTagDefaultPair(f.TimeoutExecute, f.TimeoutTransfer)},
		{long: "--connect-timeout", tag: message.HelpTagDefaultSeconds(f.TimeoutConnect)},

		{blank: true},

		{group: message.HelpGroupKey},
		{long: "--key"},

		{blank: true},

		{group: message.HelpGroupKeyTool},
		{long: "--gen-key"},
		{long: "--key-status"},
		{long: "--convert-secret"},
	}
}

// helpEntries 把排布与选项表合成帮助的行：五组，组间空行；短名、长名一律从选项表取，
// 说明按长名向 `message` 取。
func helpEntries(f execution) []message.HelpEntry {
	rows := helpRowsOf(f)
	out := make([]message.HelpEntry, 0, len(rows))
	for _, r := range rows {
		switch {
		case r.group != "":
			out = append(out, message.HelpEntry{Kind: message.EntryGroup, Desc: r.group})
		case r.blank:
			out = append(out, message.HelpEntry{Kind: message.EntryBlank})
		default:
			o, ok := lookupOption(r.long)
			if !ok {
				continue // 认不出的长名由测试拦下，屏上不排这一行
			}
			desc, ok := message.HelpDesc(r.long, f.Sudo)
			if !ok {
				continue // 没配说明的选项同上：测试先红，屏上不排这一行
			}
			out = append(out, message.HelpEntry{
				Kind: message.EntryOption, Short: o.short, Long: o.long, Tag: r.tag, Desc: desc,
			})
		}
	}
	return out
}
