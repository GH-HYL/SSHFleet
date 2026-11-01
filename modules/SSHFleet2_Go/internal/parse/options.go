package parse

import (
	"fmt"

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

// helpRows 是帮助的排布：分组、顺序、说明与标记。
//
// **选项名不在这里**——它是选项表的一部分，按长名从 `options` 里查回来。两边各写一串名字
// 迟早漂移（新增一个选项，一处改了另一处没改），这正是 16 号单 H8 指的那件事。
type helpRow struct {
	group string // 非空：一个分组标题
	blank bool   // 空行
	long  string // 选项长名，短名从 options 里查
	desc  string
	tag   string // 可选：默认值或当前配置，留空不排那一列
}

// helpRowsOf 排一张表；标记里要用的配置值就地取。
func helpRowsOf(f execution) []helpRow {
	sudoTag, noSudoDesc := "[当前配置: 关]", "这次以登录用户身份执行"
	if f.Sudo {
		sudoTag, noSudoDesc = "[当前配置: 开]", "这次以登录用户身份执行（配置是开时用它改回来）"
	}
	return []helpRow{
		{group: "模式（五选一）"},
		{long: "--command", desc: "在多台服务器上执行一条命令"},
		{long: "--script", desc: "在多台服务器上执行一个本地脚本"},
		{long: "--upload", desc: "把本地文件或目录上传到服务器"},
		{long: "--download", desc: "从服务器下载文件或目录到本地"},
		{long: "--change-password", desc: "批量改密：把清单里密码过期的账号改成同一个新密码"},

		{blank: true},

		{group: "清单与目标路径"},
		{long: "--csv-file", desc: "节点清单：CSV 文件路径，或直接写一行节点信息（必填）"},
		{long: "--path", desc: "目标路径：上传写服务器目录 / 下载写本机目录（上传、下载必填）"},

		{blank: true},

		{group: "执行选项"},
		{long: "--number", tag: "[默认: 全部]", desc: "并发数：同时操作几台，不填=全部并行"},
		{long: "--remark", desc: "备注，用作历史记录文件夹名（不填自动生成）"},
		{long: "--sudo", tag: sudoTag, desc: "这次以 root 身份执行（需目标节点已配免密 sudo）"},
		{long: "--no-sudo", desc: noSudoDesc},
		{long: "--yes", desc: "跳过所有确认直接执行（自动化用，用它之前先手动跑通一次）"},
		{long: "--answer", desc: "代填：看到触发词就自动填内容（一条写成「代填内容,触发词,触发词」），多条写进 CSV 文件再指向它"},
		{long: "--no-shell", desc: "不套工具的外壳、不设环境：命令原样下发；只能和 -c 一起用"},
		{long: "--timeout", tag: fmt.Sprintf("[默认: %d/%d]", f.TimeoutExecute, f.TimeoutTransfer), desc: "命令跑完、文件传完的最长等待（秒）"},
		{long: "--connect-timeout", tag: fmt.Sprintf("[默认: %d]", f.TimeoutConnect), desc: "连上服务器的最长等待（秒）"},

		{blank: true},

		{group: "密钥与凭据"},
		{long: "--key", desc: "密钥登录：不写=只用密码；写了=用清单或配置里的私钥"},

		{blank: true},

		{group: "密钥管理（不执行批量任务）"},
		{long: "--gen-key", desc: "生成主密钥（打开加密之后才会用到），密钥存进环境变量 SSHFLEET_KEY"},
		{long: "--key-status", desc: "查看主密钥状态：读到哪把、已存哪把、是否一致"},
		{long: "--convert-secret", desc: "转换凭据文件（后面跟要转的文件路径）：按配置里的加密开关，转成明文或密文"},
	}
}

// helpEntries 把排布与选项表合成帮助的行：五组，组间空行；短名、长名一律从选项表取。
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
			out = append(out, message.HelpEntry{
				Kind: message.EntryOption, Short: o.short, Long: o.long, Tag: r.tag, Desc: r.desc,
			})
		}
	}
	return out
}
