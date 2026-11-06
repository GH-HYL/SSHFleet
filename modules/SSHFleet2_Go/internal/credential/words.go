package credential

// 本包自己产的那几句中文。
//
// 「字只从 message 出」（ADR-0032 决定四）在这里开了唯一一个口子：本包不许 import message，
// 而这些字随错误值出去、被密钥包原样当「原因」上屏（`--convert-secret` 解不开、读不到文件）。
// 要么把字全搬进 message，就得给本包加一个「哪一步塌了」的区分类型——为一条标准库永不失败的
// 路径加一个导出类型，不划算（开发者 2026-11-06 定：允许开口，开口取最小）。
//
// **口子只有这么大**：本包只产「原因片段」与一个值（指纹「（未设置）」）。屏上的外壳——
// 前缀、路径、下一步怎么办、几行怎么排——一律归 message（`KeyConvertBlocked`、
// `KeyConfigUnreadable`、`KeyNoMasterKey` 那几句与之配套）。改口径时两处一起看。
const (
	wordKeyMissing = "主密钥没读到"
	wordBadCipher  = "不是本工具的密文格式"

	wordReadFailed    = "读不到凭据文件 %s：%w"
	wordNoCipherBuild = "建不出密码器：%v"
	wordNoGCM         = "建不出 GCM：%v"
	wordNoRandom      = "取不到随机数：%v"
	wordTooShort      = "内容长度不足，文件可能被截断"
	wordBadVersion    = "版本号 %d 认不出"
	wordBadTag        = "完整性校验不过——主密钥不匹配，或文件已损坏"
	wordDeriveFailed  = "密钥派生失败：%v"
	wordB64Length     = "base64 长度不合法"
	wordB64Charset    = "含 base64 字符集之外的字符"

	wordFingerprintEmpty = "（未设置）"
)
