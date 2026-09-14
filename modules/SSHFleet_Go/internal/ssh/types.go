// Package ssh 承载主干第 8 步的 SSH / SFTP 执行：
// 连接（主机密钥刻意跳过校验，ADR-0001）、命令与脚本执行、文件传输，
// 以及进度读写器（包裹传输的 io.Writer / io.Reader，spec D2 展开）。
package ssh

import "time"

// Config 单节点的连接与超时配置（由 batch 从节点信息与命令行参数构建）。
type Config struct {
	IP             string
	Port           int
	User           string
	Password       string
	KeyContent     string
	KeyPassphrase  string
	ConnectTimeout time.Duration
	ExecTimeout    time.Duration
}

// Result 单节点执行结果（四种模式共用一种形状：命令 / 脚本 / 上传 / 下载）。
// 退出码语义见 CONTEXT「退出码」：仅命令执行结果有退出码，传输失败保持 nil。
type Result struct {
	Seq             int
	IP              string
	Port            int
	User            string
	ConnectSuccess  bool
	ExitCode        *int
	Output          string // 命令输出 / 传输明细（明文，不再 base64）
	ConnectCostTime float64
	ExecCostTime    float64
	Error           *string

	// AuthFailure 认证失败分类：仅当「私钥与密码都配置且都尝试、且都失败」时置位
	//（文案「密钥与密码均失败」）。单一凭据失败或非认证类错误保持 nil，按原文报错。
	AuthFailure *string

	// 传输类字段（命令模式下为零值）
	TotalBytes   int64
	TotalFiles   int
	SuccessFiles int
	FailedFiles  int
}

// Progress 进度快照（事件形状由进度聚合器定义，spec M3 差异）。
type Progress struct {
	Seq             int
	IP              string
	UploadedBytes   int64
	DownloadedBytes int64
	TotalBytes      int64
	TotalFiles      int
	SuccessFiles    int
	FailedFiles     int
}

// LocalFile 待上传的本地文件条目（ssh 传输的输入契约；清理由 batch 侧收集）。
type LocalFile struct {
	Path string // 绝对路径
	Rel  string // 相对上传根目录的路径（POSIX 斜杠）：多远端就落在「-p 目标 + Rel」；单文件即文件名
	Size int64
}
