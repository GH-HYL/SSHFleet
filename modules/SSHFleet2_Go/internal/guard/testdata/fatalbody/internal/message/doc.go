package message

// 假模块：造字的那一站。类一第四条判据的样本外壳，规则表见 helpers_test.go 的 fatalbody 分支。

// FatalBody 是那一屏的正文（真工程里字段不导出，样本只要形状）。
type FatalBody struct{}

// Fatal 排一屏致命错误：正文收 FatalBody，不收裸串。
func Fatal(where string, body FatalBody, logPath string) {}

// TraceRefusal 是同一句话落进日志的形态。
func TraceRefusal(body FatalBody) {}

// RefuseUnknownOption 是一段由本包成形的正文。
func RefuseUnknownOption(name string) FatalBody { return FatalBody{} }
