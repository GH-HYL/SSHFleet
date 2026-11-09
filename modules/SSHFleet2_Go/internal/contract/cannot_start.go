package contract

// CannotStart 是「起不来」：前提缺了，连计划都生不出来。
//
// 它不是一种权力，是任何一站都可能撞上的事实，所以谁都能造——类型可见、没有构造函数。
// 它不实现 error 接口：那会在契约包里长一个 Error() 方法，与「契约只放类型与常量」打架；
// 塔尖按类型认它。
//
// 判据见 ADR-0034 决定三；它许在哪儿造，见 ADR-0032 第四条（闸门里造即报）。
type CannotStart struct {
	// Where 是老版错误屏上 [function:xxx] 那一格：撞上的是哪一环（config、cli、dangercheck…）。
	Where string

	// NoLog 为真表示这一屏末尾**不带**「日志：」那一行：配置那一步日志还没建起来
	// （老版步骤 1 在步骤 2 之前）。塔尖照它决定附不附日志路径。
	//
	// 这里刻意不装成品的话：一屏话只能由 message 造（`message.Line`），契约里连字都
	// 装不了——否则谁都能自己拼一句塞给屏幕（ADR-0032 第四条）。
	NoLog bool

	// Reason 是这一屏的原因：类别加参数，句子由 `message` 按类别成形（ADR-0044）。
	Reason CannotStartReason

	// Body 是**迁移期字段**：01／02 号单期间还有填点在往里装成品句子，03 号单末尾删掉。
	// 删完之后这一格所代表的那个口子才算关上——纸里再没有地方能放一句拼好的话。
	Body string
}

// CannotStartKind 是「起不来」的原因类别。
//
// 类别住契约（仍零函数），每个类别的模板住 `message`——纸只回答「哪一类、什么参数」，
// 那一屏的话由 `message` 一处成形（ADR-0044）。
type CannotStartKind string

// CannotStartConfigLoad 是「配置文件读不到或读不懂」：先说读的是哪一份，再附系统原话。
// 更多类别随 02／03 号单加。
const CannotStartConfigLoad CannotStartKind = "config_load"

// CannotStartReason 是那一屏要装的全部事实：一个类别，加三个具名格。
//
// 三个格按类别取用，用不上的留空：Place 是位置（文件路径、哪一份表），Extra 是这一类别
// 多要的一个原值（行号、键名、条目序号），Origin 是系统或解析器给的原话——工具不加工它。
type CannotStartReason struct {
	Kind   CannotStartKind
	Place  string
	Extra  string
	Origin string
}
