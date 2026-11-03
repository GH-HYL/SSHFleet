package contract

// Mode 是这次要干哪一种活。五种模式互斥，零值表示一个都没给。
type Mode string

const (
	ModeNone     Mode = ""
	ModeCommand  Mode = "command"
	ModeScript   Mode = "script"
	ModeUpload   Mode = "upload"
	ModeDownload Mode = "download"
	ModePasswd   Mode = "passwd"
)

// Task 是「这次干什么」那一格：一个带标签的格子，按 Plan.Mode 决定看哪一份。
//
// 五份类型不合并——命令与脚本、上传与下载共用的段落各写一遍，那是认下的重复
// （ADR-0035 决定二：现在没有事实支持合并）。
//
// Path 是 -p 的原值，**单独一格**：命令与脚本模式下它不属于任何一种任务，却仍要判
// 「-p 不能搭配 -c／-s」（老版 CheckArguments 在进任何模式分支之前就看 a.Path）。
// 上传／下载不再各存一份，免得同一份数据有两个来源。
type Task struct {
	Command  *CommandTask
	Script   *ScriptTask
	Upload   *UploadTask
	Download *DownloadTask
	Passwd   *PasswdTask

	Path string
}

// CommandTask 命令模式：一条命令原文，外加这次要喂进去的代填表。
//
// Answers 为空表示没给 `-a`（表在解析站按 `-a` 的原值解析一次放上来）。
type CommandTask struct {
	Text    string
	Answers []Answer
}

// ScriptTask 脚本模式：本地脚本路径与它的正文（正文只读一次，危险检测与下发用同一份）。
type ScriptTask struct {
	Path    string
	Body    string
	Answers []Answer
}

// UploadTask 上传模式：本机源路径，加上确认屏之前就采集好的那张表。
//
// 采集在解析站（老版 `batch/collect.go`）：软链接过滤、FIFO／设备／套接字拒收、
// 逐项可读、目录递归、相对路径、总大小，全都发生在按 y 之前——「点了 y 才失败」
// 那一类由此消失。
type UploadTask struct {
	Source string

	// Files 是可传的文件表（软链接已滤掉）。
	Files []UploadFile

	// Skipped 是被过滤掉的链接（原样留名）：结果明细里要逐个点名，不让它静默消失。
	Skipped []string

	// TopEntries 是上传根下一层的条目：确认屏那棵树按它摆，空子目录与被过滤的链接都在。
	TopEntries []UploadEntry

	// RootIsDir 是「上传根本身是不是目录」。
	//
	// 光看 TopEntries 分不出两件事：单文件上传（根就是那个文件）与空目录上传（根下没有条目）
	// 都会是空表，而确认屏这两者的样子不同（前者一行「(文件)」，后者一行「目录/」）。
	RootIsDir bool

	// TotalBytes 是这张表的总字节数。建议并发据它算，确认屏印「上传总大小」也读它——
	// 同一个数只算一处，闸门不重算。
	TotalBytes int64
}

// DownloadTask 下载模式：服务器上的源路径。本机的目标目录是 Task.Path。
type DownloadTask struct {
	RemotePath string
}

// PasswdTask 改密模式：命令行给的新密码原值，以及解析出来的明文。
//
// Raw 是**原值**（加密开着时它是密文文件的绝对路径）：报告要印的是使用者敲了什么。
// NewPassword 是 `credential` 解出来的明文，执行站拿它喂远端；报告站印之前自己调 `mask`
// 现算遮好的形态——把已经遮好的值装进纸等于凭空多一格（ADR-0032 那条结构决定）。
type PasswdTask struct {
	Raw         string
	NewPassword string
}
