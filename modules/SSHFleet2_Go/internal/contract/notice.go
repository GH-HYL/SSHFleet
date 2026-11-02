package contract

// 解析站路上要说给使用者听、但**不必中断**的那几句话。
//
// 纸上装的是「哪一类提示 + 那类提示要填的事实」，不装成品的话——话只能由 message 造
// （`message.Line`），契约里连字都装不了（ADR-0032 第四条）。塔尖拿到纸之后照 Kind 向
// message 要那一句，再交给 console。
//
// 加一类提示要动两处：这里加一个常量与它需要的事实格、message 里加一条分派。
type NoticeKind string

const (
	// NoticeManifestHeader 「第一行不是 IP，已当表头移除」——不带参数。
	NoticeManifestHeader NoticeKind = "manifest_header"

	// NoticeUnusedKey 「清单某几行配了私钥，本次没加 -k」——带行号。
	NoticeUnusedKey NoticeKind = "unused_key"
)

// Notice 一条提示。哪个 Kind 要用哪几格，写在 Kind 的注释里；用不上的格留零值。
type Notice struct {
	Kind NoticeKind

	// Rows 是清单里的行号（从 1 数，与使用者打开文件看到的行号一致）。
	// 只有 NoticeUnusedKey 用。
	Rows []int
}
