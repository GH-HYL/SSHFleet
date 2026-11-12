package message

// FatalBody 是一屏致命错误的正文：与 `Line` 同一道闸，字段不导出，只有本包造得出来。
//
// 为什么要一个类型：`Fatal` 过去收 `string`，于是「站里随手把系统原文、或者自己拼的半句话
// 贴上去」这条路一直开着——审计 08-3 那 16 处就是这么来的。收成具名类型之后，屏幕上的正文
// 只能由本包的模板成形：站里想放一句，必须先说出「哪一类、什么参数」（ADR-0047）。
//
// 它只装一句话，不装流、不装色：外壳（`[ERROR] [function:…]`）、换行、日志路径仍由 `Fatal`
// 自己加——正文与外壳分开，才不会有人把外壳也拼进正文里带两套。
type FatalBody struct {
	s string
}

// Text 是这段正文的字：给 `Fatal` 与 `TraceRefusal` 用。
//
// 它不含色码、不含外壳，与 `Line.Text` 不同——那一屏的颜色只在上外壳时加一次。
func (b FatalBody) Text() string { return b.s }

// bodyOf 把一段排好的字包成正文。
//
// 只在本包里用：外面能拿到的每一段正文都必须出自某个具名的成形函数（`Refuse*`、`Shell`、
// `CannotStartBody`），否则「哪一类」这件事就没人说得清，出口收口也就白做。
func bodyOf(s string) FatalBody { return FatalBody{s: s} }
