package message

import "strconv"

// 帮助那一屏的字（措辞迁移 04 号单）：五个分组名、二十条选项说明、标记列那几个方括号。
//
// 解析站只留排布——哪个选项、哪个分组、什么顺序；说明按长选项名来这里取。原先这些字长在
// `parse/options.go` 的 `helpRow` 里，屏上字节一个都没改。

// 帮助的五个分组标题，顺序即屏上顺序（`parse` 排表时按它们分组）。
const (
	HelpGroupMode    = "模式（五选一）"
	HelpGroupTarget  = "清单与目标路径"
	HelpGroupExec    = "执行选项"
	HelpGroupKey     = "密钥与凭据"
	HelpGroupKeyTool = "密钥管理（不执行批量任务）"
)

// helpDescs 是按长选项名取的说明。
//
// `--no-sudo` 不在表里：那一句随配置里 sudo 开着还是关着两版，见 `HelpDesc`。
var helpDescs = map[string]string{
	"--command":         "在多台服务器上执行一条命令",
	"--script":          "在多台服务器上执行一个本地脚本",
	"--upload":          "把本地文件或目录上传到服务器",
	"--download":        "从服务器下载文件或目录到本地",
	"--change-password": "批量改密：把清单里密码过期的账号改成同一个新密码",
	"--csv-file":        "节点清单：CSV 文件路径，或直接写一行节点信息（必填）",
	"--path":            "目标路径：上传写服务器目录 / 下载写本机目录（上传、下载必填）",
	"--number":          "并发数：同时操作几台，不填=全部并行",
	"--remark":          "备注，用作历史记录文件夹名（不填自动生成）",
	"--sudo":            "这次以 root 身份执行（需目标节点已配免密 sudo）",
	"--yes":             "跳过所有确认直接执行（自动化用，用它之前先手动跑通一次）",
	"--answer":          "代填：看到触发词就自动填内容（一条写成「代填内容,触发词,触发词」），多条写进 CSV 文件再指向它",
	"--no-shell":        "不套工具的外壳、不设环境：命令原样下发；只能和 -c 一起用",
	"--timeout":         "命令跑完、文件传完的最长等待（秒）",
	"--connect-timeout": "连上服务器的最长等待（秒）",
	"--key":             "密钥登录：不写=只用密码；写了=用清单或配置里的私钥",
	"--gen-key":         "生成主密钥（打开加密之后才会用到），密钥存进环境变量 SSHFLEET_KEY",
	"--key-status":      "查看主密钥状态：读到哪把、已存哪把、是否一致",
	"--convert-secret":  "转换凭据文件（后面跟要转的文件路径）：按配置里的加密开关，转成明文或密文",
}

// HelpDesc 取一个长选项的说明。
//
// sudoOn 只服务 `--no-sudo` 那一句：配置里 sudo 开着时它多一句「用它改回来」，关着就没有——
// 两种情况是两条去路，不是同一句话的两种语气。其余选项不看它。
//
// 取不到（第二个返回值为假）说明这个选项还没配文字：调用点不排那一行，`parse` 侧有条判据先红。
func HelpDesc(long string, sudoOn bool) (string, bool) {
	if long == "--no-sudo" {
		return noSudoDesc(sudoOn), true
	}
	desc, ok := helpDescs[long]
	return desc, ok
}

// noSudoDesc 是 `--no-sudo` 那一句的两版。
func noSudoDesc(sudoOn bool) string {
	if sudoOn {
		return "这次以登录用户身份执行（配置是开时用它改回来）"
	}
	return "这次以登录用户身份执行"
}

// defaultTag 是「默认值」那三个标记共用的外壳：`[默认: <值>]`——方括号与「默认」两字只写这一处。
func defaultTag(text string) string { return "[默认: " + text + "]" }

// HelpTagDefaultAll 是 `--number` 的标记：「不填就是全部并行」，数不出来所以写「全部」。
func HelpTagDefaultAll() string { return defaultTag("全部") }

// HelpTagDefaultPair 是两个超时共用的标记：`[默认: 60/300]`（执行／传输）。
func HelpTagDefaultPair(execute, transfer int) string {
	return defaultTag(strconv.Itoa(execute) + "/" + strconv.Itoa(transfer))
}

// HelpTagDefaultSeconds 是单个超时的标记：`[默认: 10]`。
func HelpTagDefaultSeconds(seconds int) string {
	return defaultTag(strconv.Itoa(seconds))
}

// HelpTagCurrentConfig 是 `--sudo` 的标记：这一次配置里是开着还是关着，`开`／`关`。
func HelpTagCurrentConfig(on bool) string {
	if on {
		return "[当前配置: 开]"
	}
	return "[当前配置: 关]"
}
