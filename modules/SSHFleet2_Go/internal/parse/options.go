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

// helpEntries 把选项表排成帮助的行：五组，组间空行；标记列的两个值来自配置那四格。
func helpEntries(f execution) []message.HelpEntry {
	sudoTag, noSudoDesc := "[当前配置: 关]", "这次以登录用户身份执行"
	if f.Sudo {
		sudoTag, noSudoDesc = "[当前配置: 开]", "这次以登录用户身份执行（配置是开时用它改回来）"
	}
	return []message.HelpEntry{
		{Kind: message.EntryGroup, Desc: "模式（五选一）"},
		{Kind: message.EntryOption, Short: "-c", Long: "--command", Desc: "在多台服务器上执行一条命令"},
		{Kind: message.EntryOption, Short: "-s", Long: "--script", Desc: "在多台服务器上执行一个本地脚本"},
		{Kind: message.EntryOption, Short: "-u", Long: "--upload", Desc: "把本地文件或目录上传到服务器"},
		{Kind: message.EntryOption, Short: "-d", Long: "--download", Desc: "从服务器下载文件或目录到本地"},
		{Kind: message.EntryOption, Long: "--change-password", Desc: "批量改密：把清单里密码过期的账号改成同一个新密码"},

		{Kind: message.EntryBlank},

		{Kind: message.EntryGroup, Desc: "清单与目标路径"},
		{Kind: message.EntryOption, Short: "-f", Long: "--csv-file", Desc: "节点清单：CSV 文件路径，或直接写一行节点信息（必填）"},
		{Kind: message.EntryOption, Short: "-p", Long: "--path", Desc: "目标路径：上传写服务器目录 / 下载写本机目录（上传、下载必填）"},

		{Kind: message.EntryBlank},

		{Kind: message.EntryGroup, Desc: "执行选项"},
		{Kind: message.EntryOption, Short: "-n", Long: "--number", Tag: "[默认: 全部]", Desc: "并发数：同时操作几台，不填=全部并行"},
		{Kind: message.EntryOption, Short: "-r", Long: "--remark", Desc: "备注，用作历史记录文件夹名（不填自动生成）"},
		{Kind: message.EntryOption, Long: "--sudo", Tag: sudoTag, Desc: "这次以 root 身份执行（需目标节点已配免密 sudo）"},
		{Kind: message.EntryOption, Long: "--no-sudo", Desc: noSudoDesc},
		{Kind: message.EntryOption, Long: "--yes", Desc: "跳过所有确认直接执行（自动化用，用它之前先手动跑通一次）"},
		{Kind: message.EntryOption, Short: "-a", Long: "--answer", Desc: "代填：看到触发词就自动填内容（一条写成「代填内容,触发词,触发词」），多条写进 CSV 文件再指向它"},
		{Kind: message.EntryOption, Long: "--no-shell", Desc: "不套工具的外壳、不设环境：命令原样下发；只能和 -c 一起用"},
		{Kind: message.EntryOption, Short: "-t", Long: "--timeout", Tag: fmt.Sprintf("[默认: %d/%d]", f.TimeoutExecute, f.TimeoutTransfer), Desc: "命令跑完、文件传完的最长等待（秒）"},
		{Kind: message.EntryOption, Short: "-T", Long: "--connect-timeout", Tag: fmt.Sprintf("[默认: %d]", f.TimeoutConnect), Desc: "连上服务器的最长等待（秒）"},

		{Kind: message.EntryBlank},

		{Kind: message.EntryGroup, Desc: "密钥与凭据"},
		{Kind: message.EntryOption, Short: "-k", Long: "--key", Desc: "密钥登录：不写=只用密码；写了=用清单或配置里的私钥"},

		{Kind: message.EntryBlank},

		{Kind: message.EntryGroup, Desc: "密钥管理（不执行批量任务）"},
		{Kind: message.EntryOption, Long: "--gen-key", Desc: "生成主密钥（打开加密之后才会用到），密钥存进环境变量 SSHFLEET_KEY"},
		{Kind: message.EntryOption, Long: "--key-status", Desc: "查看主密钥状态：读到哪把、已存哪把、是否一致"},
		{Kind: message.EntryOption, Long: "--convert-secret", Desc: "转换凭据文件（后面跟要转的文件路径）：按配置里的加密开关，转成明文或密文"},
	}
}
