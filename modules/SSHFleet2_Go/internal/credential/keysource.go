package credential

import (
	"os"
	"strings"

	"sshfleet2/internal/contract"
)

// 主密钥的两处来源（本次运行 vs 本机存着的那把），收成一个可注入的值。
//
// 三件事各是一个函数：读环境变量、读本机持久值、写本机持久值。默认实现按平台分文件
// （persist_posix.go / persist_windows.go），测试注入假件——**跑测试不许动本机**：
// 假件的 rc 路径指到临时目录，注册表那一侧根本不会被走到。
//
// 老版 `KeySource` 也是这个形状（三个字段），这里照同一形态搬，只是把「往哪个流打警告」
// 拿掉了——新工程的话只由 message 造。

// Sources 是主密钥两处来源的现状。
type Sources struct {
	// Env 是进程环境变量里的那把（本次运行实际使用）。
	Env string
	// Persisted 是本机存着的那把。
	Persisted string
	// PersistedWhere 是它的位置：POSIX 是 rc 文件绝对路径，Windows 是注册表键。
	PersistedWhere string
	// OtherKey 是另一处持久值（POSIX 的另一侧 rc 文件）；Windows 恒空。
	OtherKey   string
	OtherWhere string
}

// EnvSet 环境变量里有没有值。
func (s Sources) EnvSet() bool { return strings.TrimSpace(s.Env) != "" }

// PersistedSet 本机存没存过。
func (s Sources) PersistedSet() bool { return strings.TrimSpace(s.Persisted) != "" }

// Diverged 两处来源不一致：环境与持久值都不同，或两侧 rc 文件互相不同。
//
// 这一格决定两件事：开工前要不要提醒一句、加密方向要不要拦下来（加密不可逆）。
func (s Sources) Diverged() bool {
	if s.EnvSet() && s.PersistedSet() && strings.TrimSpace(s.Env) != strings.TrimSpace(s.Persisted) {
		return true
	}
	return s.PersistedSet() && s.OtherKey != "" && s.OtherKey != s.Persisted
}

// KeySource 是主密钥的来源。
//
// 字段都是函数而非字符串：来源是「随时可读」的东西（环境变量会变、rc 文件会被改），
// 取值应在每次询问时发生，不是构造时快照一次。
type KeySource struct {
	// EnvLookup 取进程环境变量里的主密钥。
	EnvLookup func() string
	// PersistedLookup 读本机持久值那一侧。
	PersistedLookup func() Sources
	// Persist 把一把密钥写成本机持久值。
	Persist func(key string) (contract.PersistResult, error)
	// RCFile 是当前 shell 的 rc 文件绝对路径（Windows 为空）。
	RCFile func() string
	// Host 说持久值落在哪一侧平台：拼话按它挑说法。
	Host contract.KeyHost
}

// DefaultKeySource 是运行期那一个：读真实环境、真实持久值、真实平台。
func DefaultKeySource() *KeySource {
	return &KeySource{
		EnvLookup:       func() string { return os.Getenv(EnvName) },
		PersistedLookup: readSourcesPlatform,
		Persist:         persistKey,
		RCFile:          rcFilePath,
		Host:            hostPlatform,
	}
}

// Read 汇总当前来源视图：环境变量 + 本机持久值。
//
// 两处各读各的——这样「环境变量为空但本机已保存」才能如实分辨，它正是「生成密钥后忘了
// 让它生效」的典型症状。
func (s *KeySource) Read() Sources {
	src := s.PersistedLookup()
	src.Env = strings.TrimSpace(s.EnvLookup())
	return src
}

// Inspect 判定当前状态并交出拼屏要的那份事实。
func (s *KeySource) Inspect() contract.KeyFacts {
	src := s.Read()
	return contract.KeyFacts{
		Host:         s.Host,
		State:        stateOf(src),
		EnvName:      EnvName,
		EnvSet:       src.EnvSet(),
		EnvPrint:     Fingerprint(src.Env),
		Persisted:    src.PersistedSet(),
		PersistPrint: Fingerprint(src.Persisted),
		PersistWhere: src.PersistedWhere,
		OtherPrint:   fingerprintIfSet(src.OtherKey),
		OtherWhere:   src.OtherWhere,
		RCPath:       s.RCFile(),
	}
}

// Precheck 是开工前那一问：加密开着时问一句主密钥齐不齐。
//
// 它只吃「这一次加不加密」这一个布尔——加密关着时主密钥根本用不到，查它就是纯干扰。
// 交回的事实里带状态，「要不要说话」由第二样答：正常与两处都空都不说话（老版
// `PrecheckKey` 只有三支，那两种都落到空串）。
func (s *KeySource) Precheck(encrypted bool) (contract.KeyFacts, bool) {
	facts := s.Inspect()
	if !encrypted {
		return facts, false
	}
	switch facts.State {
	case contract.KeyStateDiverged, contract.KeyStateStaleShell, contract.KeyStateUnpersisted:
		return facts, true
	}
	return facts, false
}

// Precheck 是塔尖那一问：默认来源上的同一件事。
func Precheck(encrypted bool) (contract.KeyFacts, bool) {
	return DefaultKeySource().Precheck(encrypted)
}

// Inspect 是默认来源上的同一件事（三条命令里 `--key-status` 用它）。
func Inspect() contract.KeyFacts {
	return DefaultKeySource().Inspect()
}

// stateOf 按老版 `InspectKey` 的次序判状态。
func stateOf(src Sources) contract.KeyState {
	switch {
	case src.Diverged():
		return contract.KeyStateDiverged
	case src.EnvSet() && src.PersistedSet():
		return contract.KeyStateOK
	case src.PersistedSet():
		return contract.KeyStateStaleShell
	case src.EnvSet():
		return contract.KeyStateUnpersisted
	}
	return contract.KeyStateUnset
}

// fingerprintIfSet 空值时印空串：这一行本身不出现，别印「（未设置）」。
func fingerprintIfSet(key string) string {
	if strings.TrimSpace(key) == "" {
		return ""
	}
	return Fingerprint(key)
}

// Write 把一把密钥写成本机持久值。写到哪、另一侧有没有被统一，都由它交回。
func (s *KeySource) Write(key string) (contract.PersistResult, error) {
	return s.Persist(key)
}
