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
type Task struct {
	Command  *CommandTask
	Script   *ScriptTask
	Upload   *UploadTask
	Download *DownloadTask
	Passwd   *PasswdTask
}

// CommandTask 命令模式：一条命令原文。
type CommandTask struct {
	Text string
}

// ScriptTask 脚本模式：本地脚本路径与它的正文（正文只读一次，危险检测与下发用同一份）。
type ScriptTask struct {
	Path string
	Body string
}

// UploadTask 上传模式：本机源路径与服务器上的目标目录。
type UploadTask struct {
	Source    string
	RemoteDir string
}

// DownloadTask 下载模式：服务器上的源路径与本机的目标目录。
type DownloadTask struct {
	RemotePath string
	LocalDir   string
}

// PasswdTask 改密模式：命令行给的新密码原值（加密开着时它是凭据文件路径，解密码随阶段 05）。
type PasswdTask struct {
	Raw string
}
