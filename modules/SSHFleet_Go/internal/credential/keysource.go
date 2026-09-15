// 主密钥来源（可注入的值）。
//
// 原先“读环境变量”“rc 文件在哪”“只提醒一次”三件事散在包级：取值直接 os.Getenv、
// 路径由平台文件算、提醒靠一个包级布尔。这带来两个问题：
//   - 行为的“只提醒一次”取决于调用顺序与全局置位，测试要改环境变量 + 重置全局才能复现
//   - 想验证“来源不一致”的提示，绕不开真实环境
//
// 这里把三件事收成一个值：EnvLookup 取环境变量、Persisted 读本机持久值、
// Warned 记录“本次运行是否已就来源不一致提醒过”。默认实例 DefaultKeySource()
// 就是原来的行为（读 os.Getenv + 平台持久值），平台差异仍由 build tag 文件提供；
// 测试则注入假来源，不必碰环境。
package credential

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
)

// KeySource 主密钥的来源。
//
// 字段都是函数而非字符串：来源是“随时可读”的东西（环境变量可能变、rc 文件可能被改），
// 取值应在每次询问时发生，而不是构造时快照一次。
type KeySource struct {
	// EnvLookup 取进程环境变量里的主密钥（返回值会做首尾去空白）。
	EnvLookup func() string
	// PersistedLookup 读本机持久化的来源视图（Windows 注册表 / Unix 两个 rc 文件）。
	PersistedLookup func() KeySources
	// WarnOut 警告的输出去处（默认为标准错误；测试可换成缓冲区）。
	WarnOut io.Writer

	mu     sync.Mutex
	warned bool // 本次运行是否已就「来源不一致」提醒过（替代原先的包级布尔）
}

// Read 汇总当前来源视图：环境变量 + 本机持久值。
//
// 注意 EnvLookup 与 PersistedLookup 各自独立读取——这样“环境变量为空但本机已保存”
// 这类状态才能被如实分辨（它正是「生成密钥后忘了生效」的典型症状）。
func (s *KeySource) Read() KeySources {
	src := s.PersistedLookup()
	src.Env = strings.TrimSpace(s.EnvLookup())
	return src
}

// warnDivergenceOnce 提醒来源不一致（本次运行至多一次）。
//
// 这件事挂在逐节点读凭据这条高频路径上（几万个节点就是几万次），同一句话不能刷屏；
// 预检也会说这件事（更完整的版本），故两处共用同一个“已经说过”的事实，
// 而不是共用一张只能被消费一次的票——谁先开口谁置位，提醒的有无与条数
// 都不再取决于调用顺序。这个事实现在是**这个来源值上的一个字段**，不再是包级全局。
func (s *KeySource) warnDivergenceOnce() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.warned {
		return
	}
	s.warned = true
	note := divergenceNote(s.Read())
	if note == "" {
		return
	}
	out := s.WarnOut
	if out == nil {
		out = os.Stderr
	}
	fmt.Fprintf(out, "\n%s[警告]%s %s  让两处一致：%s\n",
		colorYellow, colorReset, note, reloadHint())
}

// markWarned 标记“这件事已经说过”（预检自己会打一段更完整的提示，
// 稍后逐节点读凭据时不再重复同一件事）。
func (s *KeySource) markWarned() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.warned = true
}

// key 取当前应当使用的密钥（环境变量优先）。
func (s *KeySource) key() string { return strings.TrimSpace(s.EnvLookup()) }
