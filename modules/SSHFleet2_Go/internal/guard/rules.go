package guard

// Kind 是包在工程里的类别，六类：五站、横切、契约、密钥管理、检查、塔尖。
//
// 加一类要同时改这里、ADR-0032 第四条与 ADR-0037。
type Kind string

const (
	KindStation  Kind = "站"    // 五站：解析、闸门、执行、结算、报告
	KindCrosscut Kind = "横切"   // 八个叶子包：不产业务数据，人人可依赖
	KindContract Kind = "契约"   // 只放类型与常量
	KindKeyTool  Kind = "密钥管理" // 三条不进流水线的小命令
	KindCheck    Kind = "检查"   // 结构检查本体与红线区
	KindTip      Kind = "塔尖"   // 入口那个包
)

// Rule 是规则表的一行：这个包属于哪一类、能依赖哪些内部包。
//
// Package 写这个包在 go list 里的身份：
//   - 十六个包在 internal/ 下，写短名（parse、gate…），完整路径是 模块名 + "/internal/" + 短名；
//   - 塔尖在模块根上，写模块名本身（sshfleet2）——它的包名是 main，但 main 不是路径，拿它对不上 go list。
//
// 标准库默认可依赖，不列。塔尖对 os／os/signal／errors 的符号级限制不在这一格里表达，
// 由「依赖方向」那条检查按符号数（判据见 ADR-0032 第四条）。
type Rule struct {
	Package string
	Kind    Kind
	Deps    []string
}

// contractAndCrosscut 是契约加八个横切包：五站与密钥管理包共用的那份清单。
// 站之间不许互相依赖，所以站的那几行里没有别的站。
var contractAndCrosscut = []string{
	"contract",
	"console", "log", "ask", "archive", "exitcode", "credential", "message", "mask",
}

// crosscutDeps 是横切包允许的那一条：只许契约。
var crosscutDeps = []string{"contract"}

// thirdParty 是**模块之外**许 import 的点名白名单。
//
// 名单外的一律报红：模块之外的依赖要点名，不是为了「零依赖」，是为了供应链里每一个包
// 都有人认领。加一个名记一行（map.md 框架条目「模块之外的 import 放不放」那一行）；
// 新增一类用途（不是同一个库的另一个包）仍要开发者点头。
var thirdParty = []string{
	// 阶段 04 的 09 号单：SSH 建连。与老版同版本，开发者 2026-11-02 点头。
	"golang.org/x/crypto/ssh",
	// 阶段 04 的 13 号单：进度界面三件，与老版同版本（04 规格第八节那张名单）。
	"github.com/charmbracelet/bubbletea",
	"github.com/charmbracelet/bubbles",
	"github.com/charmbracelet/lipgloss",
}

// clockReaders 是「问现在几点」的那三个函数：谁在运行时读它们，谁就在决定「现在」。
//
// 许读的包只有三处（开发者 2026-11-02 定的口径）：执行站（耗时、超时、握手 deadline 全是
// 实时的，它必须读）、日志包（每行时间戳与轮转文件改名）、塔尖（建归档目录要「现在」
// 这一个值）。别处要时间就让它从参数进来——`archive.Create` 收的是一个现成的时刻，测试给
// 个固定值就能把目录名钉死；反过来，一处随手取「现在」，那条路径在测试里就再也对不准。
var clockReaders = []string{"Now", "Since", "Until"}

// clockHomes 是许读时钟的包（短名）。塔尖不写在这一行——它按规则表那一行的类别认，
// 因为塔尖在表里写的是模块名 `sshfleet2`，不是 `main`（见 checkClock、clockHome）。
var clockHomes = []string{"execute", "log"}

// tipDeps 是塔尖的整张清单：五站、八个横切包、契约、密钥管理包。
var tipDeps = []string{
	"parse", "gate", "execute", "settle", "report",
	"contract",
	"console", "log", "ask", "archive", "exitcode", "credential", "message", "mask",
	"keytool",
}

// rules 是包清单规则表本体，十七条，一条不少。
var rules = []Rule{
	// 五站
	{"parse", KindStation, contractAndCrosscut},
	{"gate", KindStation, contractAndCrosscut},
	{"execute", KindStation, contractAndCrosscut},
	{"settle", KindStation, contractAndCrosscut},
	{"report", KindStation, contractAndCrosscut},

	// 契约
	{"contract", KindContract, nil},

	// 八个横切。console 那行多一条 message：Print 收的是 `message.Line`——那个类型的
	// 字段不导出，「话只能由 message 造」才成立（2026-11-02 的这一笔，见 ADR-0032 末尾
	// 追加的更新记录）。message 依赖契约，console 依赖 message，不成环。
	{"console", KindCrosscut, []string{"contract", "message"}},
	{"log", KindCrosscut, crosscutDeps},
	{"ask", KindCrosscut, []string{"contract", "console", "message"}}, // 叶子之间唯一的边
	{"archive", KindCrosscut, nil},
	{"exitcode", KindCrosscut, crosscutDeps},
	{"credential", KindCrosscut, crosscutDeps},
	{"message", KindCrosscut, crosscutDeps},
	{"mask", KindCrosscut, crosscutDeps},

	// 密钥管理：只许契约与横切包，站不许依赖它
	{"keytool", KindKeyTool, contractAndCrosscut},

	// 检查：只读源码，不 import 任何一站，也不 import 契约
	{"guard", KindCheck, nil},

	// 塔尖：在模块根上，所以这一格写模块名，不写 main
	{"sshfleet2", KindTip, tipDeps},
}

// Rules 返回规则表的一份拷贝。检查读它；改返回值影响不到表本身。
func Rules() []Rule {
	out := make([]Rule, len(rules))
	copy(out, rules)
	return out
}
