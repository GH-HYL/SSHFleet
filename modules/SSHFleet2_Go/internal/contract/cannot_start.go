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

// 解析站其余十四处（03 号单）：参数用错、清单读不到、凭据预检不过三族。
const (
	// CannotStartExtraArgs 是多出来的位置参数：Value 是它跟在谁后面（认不出留空），
	// Origin 是那几段按原样拼起来。
	CannotStartExtraArgs CannotStartKind = "cli_extra_args"
	// CannotStartNoNodeParsed 是一条节点都没解析出来：一个字都不带。
	CannotStartNoNodeParsed CannotStartKind = "nodelist_empty"
	// CannotStartManifestRead 是读清单这一步栽了：Origin 是系统原话。
	CannotStartManifestRead CannotStartKind = "nodelist_read"
)

// 材料自检与读清单（03 号单）：`-s` 那份脚本、`-d` 的 `-p` 那个本机目录、`-f` 那份清单，
// 外加读清单这一步。Place 一律是那一格的原值（路径），Origin 是系统原话。
//
// `-f` 那三句在自检与读清单两处各写过一遍，现在合成一条——改口径只改一处。
const (
	// CannotStartScriptIsDir 是 `-s` 指到了目录：Place 是那个路径。
	CannotStartScriptIsDir CannotStartKind = "cli_script_is_dir"
	// CannotStartScriptUnreadable 是 `-s` 的脚本读不到：Place 是路径，Origin 是系统原话。
	CannotStartScriptUnreadable CannotStartKind = "cli_script_unreadable"
	// CannotStartScriptEmpty 是 `-s` 的脚本是空文件：Place 是路径。
	CannotStartScriptEmpty CannotStartKind = "cli_script_empty"
	// CannotStartScriptBinary 是 `-s` 的脚本里有 NUL：Place 是路径。
	CannotStartScriptBinary CannotStartKind = "cli_script_binary"
	// CannotStartScriptNotUTF8 是 `-s` 的脚本不是 UTF-8：Place 是路径。
	CannotStartScriptNotUTF8 CannotStartKind = "cli_script_not_utf8"

	// CannotStartDownloadPathUnreadable 是下载模式下 `-p` 的本机目录读不到：Place 是路径，
	// Origin 是系统原话。
	CannotStartDownloadPathUnreadable CannotStartKind = "cli_download_path_unreadable"
	// CannotStartDownloadPathNotDir 是下载模式下 `-p` 指到了文件：Place 是路径。
	CannotStartDownloadPathNotDir CannotStartKind = "cli_download_path_not_dir"

	// CannotStartCSVUnreadable 是 `-f` 的清单文件读不到：Place 是路径，Origin 是系统原话。
	CannotStartCSVUnreadable CannotStartKind = "cli_csv_unreadable"
	// CannotStartCSVEmpty 是 `-f` 的清单文件是空的：Place 是路径。
	CannotStartCSVEmpty CannotStartKind = "cli_csv_empty"
	// CannotStartCSVBinary 是 `-f` 的清单是二进制：Place 是路径。
	CannotStartCSVBinary CannotStartKind = "cli_csv_binary"

	// CannotStartManifestInlineNewline 是 `-f` 的内联值里带了换行：一个字都不带。
	CannotStartManifestInlineNewline CannotStartKind = "nodelist_inline_newline"
	// CannotStartManifestPathMissing 是 `-f` 那格既不是文件、也不像一条记录，而系统
	// 又说不出别的：Place 是那个原值。系统报得出原因时走下一条。
	CannotStartManifestPathMissing CannotStartKind = "nodelist_path_missing"
	// CannotStartManifestStatFailed 同上，但原因取系统那一句：Origin 是系统原话。
	CannotStartManifestStatFailed CannotStartKind = "nodelist_stat_failed"
	// CannotStartManifestSyntaxInline 是内联那条记录的 CSV 语法错：Index 是行号。
	CannotStartManifestSyntaxInline CannotStartKind = "nodelist_syntax_inline"
	// CannotStartManifestSyntaxFile 是清单文件的 CSV 语法错：Place 是路径，Index 是行号。
	CannotStartManifestSyntaxFile CannotStartKind = "nodelist_syntax_file"
)

// 上传源采集（03 号单）：`-u` 那棵树在确认屏之前清点一遍时撞上的毛病。
//
// 这一族走「起不来」，环节名 upload，不带合规外壳（老版那一处就没有外壳）。
const (
	// CannotStartUploadRootMissing 是 `-u` 指的东西本机上不存在：Place 是那个原值，
	// Origin 是系统原话。
	CannotStartUploadRootMissing CannotStartKind = "upload_root_missing"
	// CannotStartUploadRootIsLink 是 `-u` 指的路径本身就是软链接／快捷方式：Place 是它。
	CannotStartUploadRootIsLink CannotStartKind = "upload_root_is_link"
	// CannotStartUploadSpecialType 是撞上 FIFO／设备／socket：Place 是那个路径。
	CannotStartUploadSpecialType CannotStartKind = "upload_special_type"
	// CannotStartUploadFileUnreadable 是某个文件打不开：Place 是路径，Origin 是系统原话。
	CannotStartUploadFileUnreadable CannotStartKind = "upload_file_unreadable"
	// CannotStartUploadDirUnreadable 是那棵树走不下去：Place 是根，Origin 是系统原话。
	CannotStartUploadDirUnreadable CannotStartKind = "upload_dir_unreadable"
	// CannotStartUploadNothingLeft 是过滤完一个可传文件都不剩：Place 是根，Index 是被过滤的
	// 个数，Value 是那些名字的呈现形态（最多五个，超出带「等 N 个」）。
	CannotStartUploadNothingLeft CannotStartKind = "upload_nothing_left"
)

// 逐行报告（03b 号单）：一行一处错的那种屏，不是一句话，是一张表。
//
// 头两条是「整屏」的类别：它决定那几行怎么排（表头、缩进、收尾那句）。第三条起是「一行里
// 的一条」，只成半句——拼成一句话的活仍归 `message`，站里只交行号、原值与类别。
const (
	// CannotStartNodeRows 是逐台补全那一屏：表头「发现以下错误:」，下面一行一台。
	CannotStartNodeRows CannotStartKind = "nodelist_rows"
	// CannotStartCredRows 是凭据预检那一屏：先说共几处错，下面一行一处，末尾一句去路。
	CannotStartCredRows CannotStartKind = "nodelist_cred_rows"

	CannotStartRowIPMissing      CannotStartKind = "row_ip_missing"
	CannotStartRowIPBad          CannotStartKind = "row_ip_bad"
	CannotStartRowPortBad        CannotStartKind = "row_port_bad"
	CannotStartRowUserMissing    CannotStartKind = "row_user_missing"
	CannotStartRowKeyMissing     CannotStartKind = "row_key_missing"
	CannotStartRowPasswordMissing CannotStartKind = "row_password_missing"

	// 凭据预检的三条：Place 是那一格（密码／口令），Value 是回显（可能被遮蔽），Origin 是
	// 系统原话（没有就留空）。
	CannotStartRowCredMissing   CannotStartKind = "row_cred_missing"
	CannotStartRowCredBadCipher CannotStartKind = "row_cred_bad_cipher"
	CannotStartRowCredUnreadable CannotStartKind = "row_cred_unreadable"
	// 两条配置级的：Origin 是系统原话。
	CannotStartRowCredConfigPassword    CannotStartKind = "row_cred_config_password"
	CannotStartRowCredConfigKeyPassword CannotStartKind = "row_cred_config_key_password"
)

// 提交前那几处「取值不对」（03 号单最后一批）：代填 `-a`、改密 `--change-password`、
// 密钥命令与批量参数并行。
//
// 这三族的句子本来就住 `message`（answer.go／passwd.go／keytool.go），站里只是把它塞进纸；
// 现在纸只装类别与参数，模板不动窝，由 `message.CannotStartText` 转派过去。
const (
	CannotStartAnswerFileUnreadable  CannotStartKind = "answer_file_unreadable"
	CannotStartAnswerFileBinary      CannotStartKind = "answer_file_binary"
	CannotStartAnswerInlineNewline   CannotStartKind = "answer_inline_newline"
	CannotStartAnswerGone            CannotStartKind = "answer_gone"
	CannotStartAnswerSyntax          CannotStartKind = "answer_syntax"
	CannotStartAnswerRowInvalid      CannotStartKind = "answer_row_invalid"
	CannotStartAnswerEmpty           CannotStartKind = "answer_empty"
	CannotStartAnswerMissingTrigger  CannotStartKind = "answer_missing_trigger"
	CannotStartAnswerBadRegex        CannotStartKind = "answer_bad_regex"

	CannotStartPasswdCipherEncrypted CannotStartKind = "passwd_cipher_encrypted"
	CannotStartPasswdCipherPlain     CannotStartKind = "passwd_cipher_plain"
	CannotStartPasswdUnreadable      CannotStartKind = "passwd_unreadable"
	CannotStartPasswdUndecodable     CannotStartKind = "passwd_undecodable"
	CannotStartPasswdProblems        CannotStartKind = "passwd_problems"
	CannotStartPasswdNewline         CannotStartKind = "passwd_newline"
	CannotStartPasswdOuterSpace      CannotStartKind = "passwd_outer_space"

	// CannotStartKeyToolAlone 是一次给了两条以上密钥命令：Value 是逗号连起来的命令名。
	CannotStartKeyToolAlone CannotStartKind = "cli_keytool_alone"
	// CannotStartKeyToolWithBatch 是密钥命令与批量参数一起给：Place 是那条命令，Value 是
	// 跟着给出来的那几个选项名。
	CannotStartKeyToolWithBatch CannotStartKind = "cli_keytool_with_batch"
)

// 密钥管理那十三屏（03 号单）：生成、查状态、转换凭据。
//
// 它们的句子本来就住 `message/keytool.go`，而那一屏要的事实是一整包 `KeyFacts`（环境变量、
// 两处持久值、rc 文件路径……）——拿几个具名格凑不出来，所以纸里带一格 `Key`。
const (
	CannotStartKeyNoMasterKey        CannotStartKind = "key_no_master_key"
	CannotStartKeyConvertBlocked     CannotStartKind = "key_convert_blocked"
	CannotStartKeyConfigUnreadable   CannotStartKind = "key_config_unreadable"
	CannotStartKeyGenNoOverwrite     CannotStartKind = "key_gen_no_overwrite"
	CannotStartKeyGenSaveFailed      CannotStartKind = "key_gen_save_failed"
	CannotStartKeyConvertReadProblem CannotStartKind = "key_convert_read_problem"
	CannotStartKeyConvertDecryptFail CannotStartKind = "key_convert_decrypt_failed"
	CannotStartKeyConvertEmpty       CannotStartKind = "key_convert_empty"
	CannotStartKeyConvertWriteFail   CannotStartKind = "key_convert_write_failed"
	CannotStartKeyConvertVerifyFail  CannotStartKind = "key_convert_verify_failed"
	// 这两条只有系统原话：生不出主密钥、加不了密，下一步是看凭据包那句话。
	CannotStartKeyGenFailed     CannotStartKind = "key_gen_failed"
	CannotStartKeyEncryptFailed CannotStartKind = "key_encrypt_failed"
)

// 内部故障那两屏（03 号单）：运行上下文封不起来、两个落点开不出来。
//
// 它们原先的正文是别的包用 `fmt.Errorf` 拼好的中文——那是「站里拼句」的另一条暗道。
// 现在那两个包只交事实（哪一格、系统原话），句子回到这里。
const (
	// CannotStartRunMetaMarshal 是运行上下文序列化不出来：Origin 是 json 的原话。
	CannotStartRunMetaMarshal CannotStartKind = "runmeta_marshal"
	// CannotStartRunMetaCipher 是建不出密码器：Origin 是系统原话。
	CannotStartRunMetaCipher CannotStartKind = "runmeta_cipher"
	// CannotStartRunMetaGCM 是建不出 GCM：Origin 是系统原话。
	CannotStartRunMetaGCM CannotStartKind = "runmeta_gcm"
	// CannotStartOutputTerminal 是终端输出文件开不出来：Place 是路径，Origin 是系统原话。
	CannotStartOutputTerminal CannotStartKind = "output_terminal_open"
	// CannotStartOutputLog 是执行期日志开不出来：Place 是路径，Origin 是系统原话。
	CannotStartOutputLog CannotStartKind = "output_log_open"
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
	// CannotStartKeywordCatDuplicate 是同一块里两条分类重名：Index 是前一条的序号，Limit 是
	// 后一条的序号，Value 是那个名字。跨块同名不算——`category` 与 `exit_code_category`
	// 是两份互不相干的表。
	CannotStartKeywordCatDuplicate CannotStartKind = "keyword_category_duplicate"
	// CannotStartPasswdCatDuplicate 是改密提示词表 `category` 块里两条分类重名：同上三格。
	CannotStartPasswdCatDuplicate   CannotStartKind = "passwd_category_duplicate"
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

	// Key 只给密钥管理那几类用：那一屏要的是整包事实（哪一格环境变量、两处持久值、
	// rc 文件路径），拿几个具名格凑不出来。其余类别留零值。
	Key KeyFacts

	// Rows 只给「逐行报告」那两类用（ADR-0044，03b 号单）：多行报告不是一句话，是一张表，
	// 拿几个具名格凑不出来。其余类别留空。
	Rows []CannotStartRow
}

// CannotStartRow 是逐行报告里的一行：行号、IP 原值、这一行撞上的那几条。
//
// Line 为 0 表示这一行不挂在清单某一行上（配置级的那两条）：那一行只印条目本身，不带
// 「行 N (IP: x):」前缀。IP 是清单第 1 列原样——怎么呈现（空值顶什么）归 `message`。
type CannotStartRow struct {
	Line  int
	IP    string
	Items []CannotStartReason
}
