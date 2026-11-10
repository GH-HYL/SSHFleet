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
const CannotStartConfigLoad CannotStartKind = "config_load"

// 判据文件写坏那一族（02 号单）：按「坏在哪一层」分四层，前三层三份表共用同一类别。
const (
	// CannotStartTableRead 是读不到那一份表：Place 是表名，Extra 是路径，Origin 是系统原话。
	CannotStartTableRead CannotStartKind = "table_read"
	// CannotStartTableParse 是表读到了但解析不了：同上三格。
	CannotStartTableParse CannotStartKind = "table_parse"
	// CannotStartTableEmpty 是表里一条都没有：Place 是表名，Value 是节名（rules、step、
	// category），Origin 是路径。
	CannotStartTableEmpty CannotStartKind = "table_empty"
	// CannotStartTableUnknownField 是表里出现不认识的字：Place 是表名，Value 是那几个键，
	// Origin 是「出处」那一句（没有出处留空）。
	CannotStartTableUnknownField CannotStartKind = "table_unknown_field"
	// CannotStartCategoriesEmpty 是错误分类表一条都没有：Place 是路径。
	CannotStartCategoriesEmpty CannotStartKind = "categories_empty"
	// CannotStartPasswdEmptyStep 是改密表里一条 step 都没有：Origin 是路径。
	CannotStartPasswdEmptyStep CannotStartKind = "passwd_empty_step"
	// CannotStartPasswdNoSignal 是改密模式下「密码过期」那一分类没有关键词：一个字都不带。
	CannotStartPasswdNoSignal CannotStartKind = "passwd_no_signal"
)

// 条目层（改密提示词表、错误分类表、危险规则表各条里对不上的那一格）。
const (
	CannotStartPasswdStepNoKeywords CannotStartKind = "passwd_step_no_keywords"
	CannotStartPasswdStepEmptyItem  CannotStartKind = "passwd_step_empty_item"
	CannotStartPasswdStepBadValue   CannotStartKind = "passwd_step_bad_value"
	CannotStartPasswdStepBadMax     CannotStartKind = "passwd_step_bad_max"
	CannotStartPasswdCatNoName      CannotStartKind = "passwd_category_no_name"
	CannotStartPasswdCatEmptyItem   CannotStartKind = "passwd_category_empty_item"
	CannotStartKeywordCatNoName     CannotStartKind = "keyword_category_no_name"
	CannotStartKeywordCatEmptyItem  CannotStartKind = "keyword_category_empty_item"
	CannotStartDangerNoName         CannotStartKind = "danger_no_name"
	CannotStartDangerBadLevel       CannotStartKind = "danger_bad_level"
	CannotStartDangerExampleTooWide CannotStartKind = "danger_example_too_wide"
	CannotStartDangerReasonTooWide  CannotStartKind = "danger_reason_too_wide"
	CannotStartDangerBadEnabled     CannotStartKind = "danger_bad_enabled"
	CannotStartDangerNoRegex        CannotStartKind = "danger_no_regex"
	CannotStartDangerEmptyRegex     CannotStartKind = "danger_empty_regex"
	CannotStartDangerBadRegex       CannotStartKind = "danger_bad_regex"
)

// CannotStartReason 是那一屏要装的全部事实：一个类别，加几个具名格。
//
// 格子按类别取用，用不上的留零值：Head 是外壳句的类别常量（`message.HeadXxx`，没有外壳
// 留空），Place 是位置（路径、表名、规则名、改密表的那一节），Index 是条目序号（从 1 起，
// 0 表示这一类别不编号），Value 是这一类别要说的原值（当前值、键名、分类名），Limit 是与
// Value 成对的那个数（上限），Origin 是系统或解析器给的原话——工具不加工它。
// 句子怎么拼只住在 message。
type CannotStartReason struct {
	Kind   CannotStartKind
	Head   string
	Place  string
	Index  int
	Value  string
	Limit  int
	Origin string
}
