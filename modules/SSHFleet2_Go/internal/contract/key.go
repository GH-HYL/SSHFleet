package contract

// 三条不进流水线的小命令，与它们要的那点事实（ADR-0032 第五条「一条小命令」）。
//
// 类型与常量住这里，是因为造话的 `message` 与做事的 `keytool` 都要认同一套词——而
// `message` 只许依赖契约。函数一个不放：契约包里出现行为就长成抽屉（ADR-0032 第二条）。

// KeyTool 是哪一条密钥管理命令。
type KeyTool string

const (
	// KeyToolGen 生成主密钥并写成本机持久值。
	KeyToolGen KeyTool = "gen-key"
	// KeyToolStatus 看主密钥的两处来源与状态。
	KeyToolStatus KeyTool = "key-status"
	// KeyToolConvert 把一份凭据文件在明文与密文之间转一次。
	KeyToolConvert KeyTool = "convert-secret"
)

// KeyRun 是一条密钥命令要的全部事实：哪一条、路径原值、`--yes` 给没给。
//
// **只装这三样**：它不是第一张纸，不该背上那张纸的重量（ADR-0035 那条边界同样适用）。
type KeyRun struct {
	Which KeyTool
	Path  string // `--convert-secret` 的原值，原样不 trim
	Yes   bool
}

// KeyHost 是主密钥持久值落在哪一侧平台。拼话要按它挑说法，所以它是一条事实而不是
// 一个编译期分支——`message` 不判平台，只看这一格。
type KeyHost string

const (
	// KeyHostPOSIX 持久值是登录 shell 的 rc 文件。
	KeyHostPOSIX KeyHost = "posix"
	// KeyHostWindows 持久值是注册表 HKCU\Environment。
	KeyHostWindows KeyHost = "windows"
)

// KeyState 是主密钥两处来源的组合状态。判定次序与老版逐条相同：
// 不一致 → 两处都有 → 只剩持久值 → 只剩环境变量 → 都空。
type KeyState string

const (
	// KeyStateOK 两处一致，正常可用。
	KeyStateOK KeyState = "ok"
	// KeyStateDiverged 两处都有但不同。
	KeyStateDiverged KeyState = "diverged"
	// KeyStateStaleShell 已保存但当前终端读不到。
	KeyStateStaleShell KeyState = "stale_shell"
	// KeyStateUnpersisted 当前终端有值但没持久化。
	KeyStateUnpersisted KeyState = "unpersisted"
	// KeyStateUnset 两处都空。
	KeyStateUnset KeyState = "unset"
)

// KeyFacts 是主密钥的现状：拼屏与落日志要用的全部事实。
//
// 指纹只在这里以「八位十六进制」的形态出现——密钥本体一个字都不装，它也就到不了屏幕与日志
// （老版 `Fingerprint` 那条纪律）。
type KeyFacts struct {
	Host  KeyHost
	State KeyState

	// EnvName 是主密钥那一格环境变量的名字：提示里要点名它，才说得清「本次运行读的是哪一格」。
	EnvName string

	EnvSet       bool
	EnvPrint     string // 环境变量那把的指纹；没有时为空
	Persisted    bool
	PersistPrint string
	PersistWhere string
	OtherPrint   string // 另一处持久值（POSIX 的另一侧 rc 文件）；Windows 恒为空
	OtherWhere   string

	// RCPath 是当前 shell 的 rc 文件绝对路径（POSIX 才有）：那句「source 一下」要指着它。
	RCPath string
}

// CredFileProblem 是转换前读凭据文件时认出的毛病：哪种毛病决定说哪句话。
type CredFileProblem string

// PersistResult 是一次「把主密钥写成本机持久值」的结果。
//
// 只装事实：写到哪、另一侧 rc 文件有没有被统一。怎么跟使用者说归 message。
type PersistResult struct {
	Where        string // 写到哪儿（rc 文件绝对路径，或注册表那一格）
	OtherUnified string // 被一并改成同一把的另一侧 rc 文件；空表示没有
}

const (
	// CredFileUnreadable 文件读不到（不在、没权限）。
	CredFileUnreadable CredFileProblem = "unreadable"
	// CredFileEmpty 文件是空的。
	CredFileEmpty CredFileProblem = "empty"
	// CredFileBinary 文件含二进制数据。
	CredFileBinary CredFileProblem = "binary"
	// CredFileMultiline 文件有多行内容而它不是密文。
	CredFileMultiline CredFileProblem = "multiline"
)
