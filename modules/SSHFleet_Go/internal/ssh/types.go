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

// 中间步骤的语义名（StepResult.Name 的取值）。来源冻结在三类工具内部的伴随命令
// （W10，作者 2026-10-04）：以后执行路径里出现新的中间命令，必须连这份清单一起补议，
// 不许顺手 append。裸命令原文只出现在呈现层的映射表里，这里不放。
const (
	StepPrecheck  = "precheck"  // 下载预检查，命令 test -e
	StepEnumerate = "enumerate" // 远程文件枚举，命令 find
	StepPasswd    = "passwd"    // 改密收尾的空命令，命令 :
)

// StepResult 一条中间步骤命令的退出码（带语义名）。中间步骤不止一条：
// 下载侧的 test -e 预检查与 find 枚举、改密下发的空命令 `:` 都在这里，
// 它们的退出码不进 ExitCode（result-verdict spec D30 / D33）。
type StepResult struct {
	Name     string // 步骤语义名（StepPrecheck / StepEnumerate / StepPasswd），呈现层据此映射中文
	ExitCode *int   // 该步骤自己的退出码；拿不到（超时 / 中断）为 nil
}

// Result 单节点执行结果。结构体分两区，谁都不碰对方的格子：
//
//   - 事实区：执行侧写，只此一次，已写下的值不许再改（哪个模式写哪几格见各字段注释）；
//   - 结论区：只由结果判定（internal/verdict.Judge）写，执行侧一个字都不碰——
//     成败、分类、定论退出码全工程只在这里产生一次。
//
// 退出码语义见 CONTEXT「退出码」。
type Result struct {
	// —— 事实区 ——
	Seq             int
	IP              string
	Port            int
	User            string
	AuthMethod      string // 登录方式（密钥 / 密码 / 密钥/密码）：执行期日志留痕用，不参与分类
	ConnectSuccess  bool
	ConnectCostTime float64
	ExecCostTime    float64
	Output          string // 命令输出 / 传输明细：明文，采集侧已去整块首尾空白行，呈现层原样用
	Error           *string

	// SessionBegun 命令是否真的下发成功（命令 / 脚本 / 代填 / 改密写；传输模式不写）。
	// 不再靠 Error 是否为空反猜。
	SessionBegun bool
	// TimedOut 到点收场（-t）。超时文案仍照实写进 Error（它是报错原文），
	// 但分类不再依赖那句话——成败在这里就有。
	TimedOut bool
	// Canceled 外部中断（Ctrl+C / 上级取消）。与 TimedOut 同理，Error 照实、成败在这里。
	Canceled bool
	// CommandExitCode **目的那条命令**的退出码（命令 / 脚本 / 代填写它；有就有、没有就 nil）。
	// 改密没有目的命令（下发的 `:` 是中间步骤，进 Steps），传输没有命令，两者恒为 nil。
	CommandExitCode *int
	// Steps 中间步骤命令各自的退出码（D30）。现有三条来源：下载预检 `test -e`、
	// 远程文件枚举 `find`、改密的 `:`。
	Steps []StepResult
	// ServerBanner 认证阶段服务端提示原文，无条件写（D34）。
	// 只参与判定匹配与失败行的呈现，正常行不显示。
	ServerBanner string
	// AnswersMissed 哪几条代填没送出去（序号 1 起；代填写）。
	AnswersMissed []int
	// AnswersExhausted 代填已全部送出（用于区分"表空了还超时"；代填写）。
	AnswersExhausted bool
	// AbortLine 中止词命中的原文行（代填写）。
	AbortLine string
	// EnterSignalSeen 改密：整场出现过"要求改密 / 密码已过期"的服务端信号没有。
	EnterSignalSeen bool
	// PasswdEarlyClose 改密：远端当场重问（早收场）。
	PasswdEarlyClose bool

	// 传输类字段（命令模式下为零值）
	TotalBytes   int64
	TotalFiles   int
	SuccessFiles int
	FailedFiles  int

	// —— 结论区（internal/verdict.Judge 写，只此一次）——
	// Verdict 成败二态：success / other（其他即失败）。
	Verdict string
	// Category 失败时的分类名；成功时留空（成功行的中文分类由统计/呈现侧按模式给）。
	Category string
	// ExitCode 定论退出码：判定最后一步赋值或保持 nil。
	// 不是新增字段——本轮的改变是换主人：执行侧从此一个字都不写它。
	ExitCode *int
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
